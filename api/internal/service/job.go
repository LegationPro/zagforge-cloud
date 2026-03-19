package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	dbpkg "github.com/LegationPro/zagforge-mvp-impl/api/internal/db"
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/engine"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/jobtoken"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/pgerr"
	github "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider/github"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/store"
)

// JobService orchestrates job creation with deduplication.
// It satisfies webhook.PushHandler.
type JobService struct {
	db       *dbpkg.DB
	log      *zap.Logger
	enqueuer engine.TaskEnqueuer
	signer   *jobtoken.Signer
}

func NewJobService(db *dbpkg.DB, log *zap.Logger, enqueuer engine.TaskEnqueuer, signer *jobtoken.Signer) *JobService {
	return &JobService{db: db, log: log, enqueuer: enqueuer, signer: signer}
}

// HandlePush persists a new queued job for the push event (with dedup).
// Dedup strategy: if a queued job already exists for this branch, update its
// commit_sha in place — the existing Cloud Tasks task will pick up the latest
// SHA when the worker calls /internal/jobs/start. If a running job exists (but
// no queued), a new queued job is created and enqueued.
// If the repo is not registered, the event is silently dropped.
func (s *JobService) HandlePush(ctx context.Context, event github.WebhookEvent, deliveryID string) error {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			s.log.Warn("rollback error", zap.Error(err))
		}
	}()

	qtx := store.New(tx)

	// 1. Look up registered repo — drop silently if not found.
	repo, err := qtx.GetRepoByGithubID(ctx, event.RepoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.log.Warn("repo not registered, skipping",
				zap.Int64("github_id", event.RepoID),
				zap.String("name", event.RepoName),
				zap.Int64("installation", event.InstallationID),
			)
			return nil
		}
		return fmt.Errorf("get repo: %w", err)
	}

	// 2. Acquire per-(repo, branch) advisory lock for the duration of this transaction.
	if _, err := tx.Exec(ctx,
		"SELECT pg_advisory_xact_lock(hashtext($1::text || ':' || $2)::bigint)",
		repo.ID, event.Branch,
	); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}

	// 3. Check active jobs — update-in-place or create new.
	active, err := qtx.GetActiveJobsForBranch(ctx, store.GetActiveJobsForBranchParams{
		RepoID: repo.ID,
		Branch: event.Branch,
	})
	if err != nil {
		return fmt.Errorf("get active jobs: %w", err)
	}

	// If a queued job exists, update its commit_sha in place. The existing
	// Cloud Tasks task (or poller) will pick up the latest SHA.
	for _, j := range active {
		if j.Status == store.JobStatusQueued {
			if err := qtx.UpdateJobCommitSHA(ctx, store.UpdateJobCommitSHAParams{
				ID:        j.ID,
				CommitSha: event.CommitSHA,
			}); err != nil {
				return fmt.Errorf("update job commit sha: %w", err)
			}

			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("commit tx: %w", err)
			}

			s.log.Info("job updated in place",
				zap.String("job_id", j.ID.String()),
				zap.String("repo", event.RepoName),
				zap.String("branch", event.Branch),
				zap.String("commit", event.CommitSHA),
			)
			return nil
		}
	}

	// No queued job — create a new one.
	job, err := qtx.CreateJob(ctx, store.CreateJobParams{
		RepoID:    repo.ID,
		Branch:    event.Branch,
		CommitSha: event.CommitSHA,
		Column4:   deliveryID,
	})
	if err != nil {
		if pgerr.IsCode(err, pgerr.UniqueViolation) {
			return nil // duplicate delivery, no-op
		}
		return fmt.Errorf("create job: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	s.log.Info("job queued",
		zap.String("job_id", job.ID.String()),
		zap.String("repo", event.RepoName),
		zap.String("branch", event.Branch),
		zap.String("commit", event.CommitSHA),
	)

	// Enqueue AFTER commit so the task doesn't reference an uncommitted job.
	// On failure, log and move on — the watchdog will catch orphaned jobs.
	jobID := job.ID.String()
	token := s.signer.Sign(jobID)
	if err := s.enqueuer.Enqueue(ctx, jobID, token); err != nil {
		s.log.Error("failed to enqueue task (watchdog will recover)",
			zap.String("job_id", jobID),
			zap.Error(err),
		)
	}

	return nil
}
