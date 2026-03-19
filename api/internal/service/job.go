package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	dbpkg "github.com/LegationPro/zagforge-mvp-impl/api/internal/db"
	github "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider/github"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/store"
)

// JobService orchestrates job creation with deduplication.
// It satisfies handler.pushHandler.
type JobService struct {
	db  *dbpkg.DB
	log *zap.Logger
}

func NewJobService(db *dbpkg.DB, log *zap.Logger) *JobService {
	return &JobService{db: db, log: log}
}

// HandlePush persists a new queued job for the push event (with dedup).
// The job will be picked up by a separate worker process.
// If the repo is not registered, the event is silently dropped.
func (s *JobService) HandlePush(ctx context.Context, event github.WebhookEvent, deliveryID string) error {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, sql.ErrTxDone) && err.Error() != "tx is closed" {
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

	// 3. Supersede any existing queued jobs for this branch.
	active, err := qtx.GetActiveJobsForBranch(ctx, store.GetActiveJobsForBranchParams{
		RepoID: repo.ID,
		Branch: event.Branch,
	})
	if err != nil {
		return fmt.Errorf("get active jobs: %w", err)
	}
	for _, j := range active {
		if j.Status == store.JobStatusQueued {
			if err := qtx.MarkJobSuperseded(ctx, j.ID); err != nil {
				return fmt.Errorf("mark job superseded: %w", err)
			}
		}
	}

	// 4. Insert new queued job.
	job, err := qtx.CreateJob(ctx, store.CreateJobParams{
		RepoID:    repo.ID,
		Branch:    event.Branch,
		CommitSha: event.CommitSHA,
		Column4:   deliveryID,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
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
	return nil
}
