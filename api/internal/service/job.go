package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	dbpkg "github.com/LegationPro/zagforge-mvp-impl/api/internal/db"
	dbsqlc "github.com/LegationPro/zagforge-mvp-impl/api/internal/db/sqlc"
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/runner"
	github "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider/github"
)

// JobService orchestrates job creation with deduplication.
// It satisfies handler.pushHandler.
type JobService struct {
	db     *dbpkg.DB
	runner *runner.Runner
	log    *zap.Logger
}

func NewJobService(db *dbpkg.DB, runner *runner.Runner, log *zap.Logger) *JobService {
	return &JobService{db: db, runner: runner, log: log}
}

// HandlePush persists a new queued job for the push event (with dedup) then dispatches it.
// If the repo is not registered, the event is silently dropped.
// If deliveryID is empty (header absent), it is stored as NULL.
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

	qtx := dbsqlc.New(tx)

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
	active, err := qtx.GetActiveJobsForBranch(ctx, dbsqlc.GetActiveJobsForBranchParams{
		RepoID: repo.ID,
		Branch: event.Branch,
	})
	if err != nil {
		return fmt.Errorf("get active jobs: %w", err)
	}
	for _, j := range active {
		if j.Status == dbsqlc.JobStatusQueued {
			if err := qtx.MarkJobSuperseded(ctx, j.ID); err != nil {
				return fmt.Errorf("mark job superseded: %w", err)
			}
		}
	}

	// 4. Insert new queued job.
	job, err := qtx.CreateJob(ctx, dbsqlc.CreateJobParams{
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

	// 5. Dispatch outside the transaction with detached context.
	s.dispatch(job.ID, repo.ID, event)
	return nil
}

// dispatch runs the job in a background goroutine, managing status transitions and snapshot creation.
func (s *JobService) dispatch(jobID, repoID pgtype.UUID, event github.WebhookEvent) {
	s.runner.GoWait(func() {
		ctx := context.Background()

		// Mark running.
		if err := s.db.Queries.UpdateJobStatus(ctx, dbsqlc.UpdateJobStatusParams{
			ID:     jobID,
			Status: dbsqlc.JobStatusRunning,
		}); err != nil {
			s.log.Error("failed to mark job running", zap.Error(err))
			return
		}

		result, err := s.runner.Run(ctx, event)
		if err != nil {
			s.log.Error("job failed",
				zap.String("repo", event.RepoName),
				zap.String("branch", event.Branch),
				zap.String("commit", event.CommitSHA),
				zap.Error(err),
			)
			s.db.Queries.UpdateJobStatus(ctx, dbsqlc.UpdateJobStatusParams{
				ID:     jobID,
				Status: dbsqlc.JobStatusFailed,
				ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
			})
			return
		}

		// Create snapshot.
		_, snapErr := s.db.Queries.InsertSnapshot(ctx, dbsqlc.InsertSnapshotParams{
			RepoID:          repoID,
			JobID:           jobID,
			Branch:          event.Branch,
			CommitSha:       event.CommitSHA,
			GcsPath:         result.ReportsDir,
			SnapshotVersion: 1,
			ZigzagVersion:   result.ZigzagVersion,
			SizeBytes:       result.SizeBytes,
		})
		if snapErr != nil {
			s.log.Error("failed to insert snapshot", zap.Error(snapErr))
		}

		// Mark succeeded.
		if err := s.db.Queries.UpdateJobStatus(ctx, dbsqlc.UpdateJobStatusParams{
			ID:     jobID,
			Status: dbsqlc.JobStatusSucceeded,
		}); err != nil {
			s.log.Error("failed to mark job succeeded", zap.Error(err))
		}
	})
}
