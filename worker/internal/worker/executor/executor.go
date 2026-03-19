package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/LegationPro/zagforge/shared/go/runner"
	"github.com/LegationPro/zagforge/shared/go/storage"
	"github.com/LegationPro/zagforge/worker/internal/apiclient"
)

// Executor runs a claimed job: start callback → clone → zigzag → GCS upload → complete callback.
type Executor struct {
	api    *apiclient.Client
	gcs    *storage.Client
	runner *runner.Runner
	log    *zap.Logger
}

func NewExecutor(api *apiclient.Client, gcs *storage.Client, runner *runner.Runner, log *zap.Logger) *Executor {
	return &Executor{api: api, gcs: gcs, runner: runner, log: log}
}

// Execute runs the full job lifecycle for a claimed job.
func (e *Executor) Execute(ctx context.Context, jobID string, orgID string, repoID string) {
	if e.api == nil {
		e.log.Warn("executor: api client not configured, skipping job", zap.String("job_id", jobID))
		return
	}

	// 1. Call start callback — gets clone info.
	info, err := e.api.Start(ctx, jobID)
	if err != nil {
		e.log.Error("start callback failed", zap.String("job_id", jobID), zap.Error(err))
		e.completeFailed(ctx, jobID, fmt.Sprintf("start callback: %v", err))
		return
	}

	e.log.Info("job started",
		zap.String("job_id", jobID),
		zap.String("repo", info.RepoFullName),
		zap.String("branch", info.Branch),
		zap.String("commit", info.CommitSHA),
	)

	// 2. Clone and run zigzag.
	result, err := e.runner.RunWithToken(ctx, runner.JobInfo{
		RepoFullName:   info.RepoFullName,
		Branch:         info.Branch,
		CommitSHA:      info.CommitSHA,
		CloneToken:     info.CloneToken,
		InstallationID: info.InstallationID,
	})
	if err != nil {
		e.log.Error("job execution failed", zap.String("job_id", jobID), zap.Error(err))
		e.completeFailed(ctx, jobID, err.Error())
		return
	}

	// 3. Upload snapshot to GCS.
	gcsPath := storage.SnapshotPath(orgID, repoID, info.CommitSHA)
	snapshotData, err := readReportFile(result.ReportsDir)
	if err != nil {
		e.log.Error("read report failed", zap.String("job_id", jobID), zap.Error(err))
		e.completeFailed(ctx, jobID, fmt.Sprintf("read report: %v", err))
		return
	}

	if err := e.gcs.Upload(ctx, gcsPath, snapshotData); err != nil {
		e.log.Error("gcs upload failed", zap.String("job_id", jobID), zap.Error(err))
		e.completeFailed(ctx, jobID, fmt.Sprintf("gcs upload: %v", err))
		return
	}

	// 4. Call complete callback with success.
	if err := e.api.Complete(ctx, apiclient.CompleteRequest{
		JobID:         jobID,
		Status:        "succeeded",
		SnapshotPath:  gcsPath,
		ZigzagVersion: result.ZigzagVersion,
		SizeBytes:     result.SizeBytes,
	}); err != nil {
		e.log.Error("complete callback failed", zap.String("job_id", jobID), zap.Error(err))
		return
	}

	e.log.Info("job succeeded",
		zap.String("job_id", jobID),
		zap.String("gcs_path", gcsPath),
		zap.String("zigzag_version", result.ZigzagVersion),
		zap.Int64("size_bytes", result.SizeBytes),
	)
}

func (e *Executor) completeFailed(ctx context.Context, jobID string, errMsg string) {
	if err := e.api.Complete(ctx, apiclient.CompleteRequest{
		JobID:        jobID,
		Status:       "failed",
		ErrorMessage: errMsg,
	}); err != nil {
		e.log.Error("failed to report job failure",
			zap.String("job_id", jobID),
			zap.Error(err),
		)
	}
}

// readReportFile reads the main report.json from the reports directory.
func readReportFile(reportsDir string) ([]byte, error) {
	return os.ReadFile(filepath.Join(reportsDir, "report.json"))
}
