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

// JobRecorder is the subset of store.Queries the executor needs.
type JobRecorder interface {
	UpdateJobStatus(ctx context.Context, arg store.UpdateJobStatusParams) error
	InsertSnapshot(ctx context.Context, arg store.InsertSnapshotParams) (store.Snapshot, error)
}

// Executor runs a claimed job: clone → zigzag → snapshot → status update.
type Executor struct {
	recorder JobRecorder
	runner   *runner.Runner
	log      *zap.Logger
}

func NewExecutor(recorder JobRecorder, runner *runner.Runner, log *zap.Logger) *Executor {
	return &Executor{recorder: recorder, runner: runner, log: log}
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
		if statusErr := e.recorder.UpdateJobStatus(ctx, store.UpdateJobStatusParams{
			ID:           job.ID,
			Status:       store.JobStatusFailed,
			ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
		}); statusErr != nil {
			e.log.Error("failed to update job status", zap.String("job_id", job.ID.String()), zap.Error(statusErr))
		}
		return
	}

	if _, snapErr := e.recorder.InsertSnapshot(ctx, store.InsertSnapshotParams{
		RepoID:          job.RepoID,
		JobID:           job.ID,
		Branch:          job.Branch,
		CommitSha:       job.CommitSha,
		GcsPath:         result.ReportsDir,
		SnapshotVersion: 1,
		ZigzagVersion:   result.ZigzagVersion,
		SizeBytes:       result.SizeBytes,
	}); snapErr != nil {
		e.log.Error("failed to insert snapshot", zap.String("job_id", job.ID.String()), zap.Error(snapErr))
	}

	if statusErr := e.recorder.UpdateJobStatus(ctx, store.UpdateJobStatusParams{
		ID:     job.ID,
		Status: store.JobStatusSucceeded,
	}); statusErr != nil {
		e.log.Error("failed to update job status", zap.String("job_id", job.ID.String()), zap.Error(statusErr))
	}

	e.log.Info("job succeeded",
		zap.String("job_id", job.ID.String()),
		zap.String("repo", repo.FullName),
		zap.String("zigzag_version", result.ZigzagVersion),
		zap.Int64("size_bytes", result.SizeBytes),
	)
}
