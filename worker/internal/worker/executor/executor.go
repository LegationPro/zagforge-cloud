package executor

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	githubprovider "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider/github"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/runner"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/store"
)

// Executor runs a claimed job: clone → zigzag → snapshot → status update.
type Executor struct {
	queries *store.Queries
	runner  *runner.Runner
	log     *zap.Logger
}

func NewExecutor(queries *store.Queries, runner *runner.Runner, log *zap.Logger) *Executor {
	return &Executor{queries: queries, runner: runner, log: log}
}

func (e *Executor) Execute(ctx context.Context, job store.Job, repo store.GetRepoForJobRow) {
	cloneURL := fmt.Sprintf("https://github.com/%s.git", repo.FullName)

	result, err := e.runner.Run(ctx, githubprovider.WebhookEvent{
		RepoID:         repo.GithubRepoID,
		RepoName:       repo.FullName,
		CloneURL:       cloneURL,
		Branch:         job.Branch,
		CommitSHA:      job.CommitSha,
		InstallationID: repo.InstallationID,
	})
	if err != nil {
		e.log.Error("job failed",
			zap.String("job_id", job.ID.String()),
			zap.String("repo", repo.FullName),
			zap.Error(err),
		)
		err = e.queries.UpdateJobStatus(ctx, store.UpdateJobStatusParams{
			ID:           job.ID,
			Status:       store.JobStatusFailed,
			ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
		})
		if err != nil {
			e.log.Error("failed to update job status", zap.String("job_id", job.ID.String()), zap.Error(err))
		}
		return
	}

	_, snapErr := e.queries.InsertSnapshot(ctx, store.InsertSnapshotParams{
		RepoID:          job.RepoID,
		JobID:           job.ID,
		Branch:          job.Branch,
		CommitSha:       job.CommitSha,
		GcsPath:         result.ReportsDir,
		SnapshotVersion: 1,
		ZigzagVersion:   result.ZigzagVersion,
		SizeBytes:       result.SizeBytes,
	})
	if snapErr != nil {
		e.log.Error("failed to insert snapshot", zap.String("job_id", job.ID.String()), zap.Error(snapErr))
	}

	err = e.queries.UpdateJobStatus(ctx, store.UpdateJobStatusParams{
		ID:     job.ID,
		Status: store.JobStatusSucceeded,
	})
	if err != nil {
		e.log.Error("failed to update job status", zap.String("job_id", job.ID.String()), zap.Error(err))
	}

	e.log.Info("job succeeded",
		zap.String("job_id", job.ID.String()),
		zap.String("repo", repo.FullName),
		zap.String("zigzag_version", result.ZigzagVersion),
		zap.Int64("size_bytes", result.SizeBytes),
	)
}
