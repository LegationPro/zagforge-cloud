package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	github "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider/github"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// RepoCloner is the subset of provider.Worker the runner needs.
type RepoCloner interface {
	GenerateCloneToken(ctx context.Context, installationID int64) (string, error)
	CloneRepo(ctx context.Context, repoURL, ref, token, dst string) error
}

// Config holds runner settings, all resolvable from environment variables.
type Config struct {
	WorkspaceDir string        // base dir for temporary clone directories
	ZigzagBin    string        // path to the zigzag binary
	ReportsDir   string        // absolute path where zigzag writes reports
	JobTimeout   time.Duration // hard timeout per job (0 = no timeout)
}

// Result holds metadata about a successful zigzag run.
type Result struct {
	ReportsDir    string
	ZigzagVersion string
	SizeBytes     int64
}

// Runner clones a repo, runs zigzag, then cleans up the temporary clone.
type Runner struct {
	cloner   RepoCloner
	cfg      Config
	log      *zap.Logger
	wg       sync.WaitGroup
	inflight atomic.Int64
}

func New(cloner RepoCloner, cfg Config, log *zap.Logger) *Runner {
	return &Runner{cloner: cloner, cfg: cfg, log: log}
}

// GoWait runs fn in a goroutine tracked by the runner's WaitGroup.
// Used by the service layer to run jobs while preserving graceful shutdown.
func (r *Runner) GoWait(fn func()) {
	r.inflight.Add(1)
	r.wg.Go(func() {
		defer r.inflight.Add(-1)
		fn()
	})
}

// InFlight returns the number of currently running jobs.
func (r *Runner) InFlight() int64 {
	return r.inflight.Load()
}

// Wait blocks until all in-flight jobs complete. Call during graceful shutdown.
func (r *Runner) Wait() {
	r.wg.Wait()
}

// Drain waits for in-flight jobs to finish, logging progress every tick.
// If the hard timeout is reached, it returns an error with the remaining count.
func (r *Runner) Drain(timeout time.Duration, tick time.Duration) error {
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	deadline := time.After(timeout)

	for {
		select {
		case <-done:
			r.log.Info("all jobs drained")
			return nil
		case <-ticker.C:
			n := r.inflight.Load()
			r.log.Info("draining in-flight jobs", zap.Int64("remaining", n))
		case <-deadline:
			n := r.inflight.Load()
			if n == 0 {
				return nil
			}
			r.log.Error("drain timeout reached, abandoning jobs", zap.Int64("remaining", n))
			return fmt.Errorf("drain timeout: %d jobs still running", n)
		}
	}
}

// JobInfo holds the parameters for a job where the clone token is already available
// (e.g. provided by the start callback).
type JobInfo struct {
	RepoFullName   string
	Branch         string
	CommitSHA      string
	CloneToken     string
	InstallationID int64
}

// RunWithToken executes a job using a pre-generated clone token (from the start callback).
func (r *Runner) RunWithToken(ctx context.Context, info JobInfo) (*Result, error) {
	if r.cfg.JobTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.cfg.JobTimeout)
		defer cancel()
	}

	r.log.Info("starting job",
		zap.String("repo", info.RepoFullName),
		zap.String("branch", info.Branch),
		zap.String("commit", info.CommitSHA),
	)

	cloneURL := fmt.Sprintf("https://github.com/%s.git", info.RepoFullName)

	return r.cloneAndRun(ctx, cloneURL, info.Branch, info.CommitSHA, info.CloneToken, info.RepoFullName)
}

// Run executes the full job: generate token → clone → zigzag → cleanup.
func (r *Runner) Run(ctx context.Context, event github.WebhookEvent) (*Result, error) {
	if r.cfg.JobTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.cfg.JobTimeout)
		defer cancel()
	}

	r.log.Info("starting job",
		zap.String("repo", event.RepoName),
		zap.String("branch", event.Branch),
		zap.String("commit", event.CommitSHA),
	)

	token, err := r.cloner.GenerateCloneToken(ctx, event.InstallationID)
	if err != nil {
		return nil, fmt.Errorf("generate clone token: %w", err)
	}

	return r.cloneAndRun(ctx, event.CloneURL, event.Branch, event.CommitSHA, token, event.RepoName)
}

// cloneAndRun handles the shared logic: clone → zigzag → collect results → cleanup.
func (r *Runner) cloneAndRun(ctx context.Context, cloneURL, branch, commitSHA, token, repoName string) (*Result, error) {
	if err := os.MkdirAll(r.cfg.WorkspaceDir, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace dir: %w", err)
	}

	workDir, err := os.MkdirTemp(r.cfg.WorkspaceDir, "job-*")
	if err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}

	defer func(workDir string) {
		if err := os.RemoveAll(workDir); err != nil {
			r.log.Warn("failed to remove work dir", zap.String("path", workDir), zap.Error(err))
		}
	}(workDir)

	repoDir := filepath.Join(workDir, "repo")
	if err := r.cloner.CloneRepo(ctx, cloneURL, branch, token, repoDir); err != nil {
		return nil, fmt.Errorf("clone repo: %w", err)
	}

	r.log.Info("running zigzag",
		zap.String("repo", repoName),
		zap.String("reports_dir", r.cfg.ReportsDir),
	)
	cmd := exec.CommandContext(ctx, r.cfg.ZigzagBin, "run", "--no-watch", "--output-dir", r.cfg.ReportsDir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("zigzag run: %w: %s", err, out)
	}

	result, err := collectResult(r.cfg.ReportsDir)
	if err != nil {
		return nil, fmt.Errorf("collect result: %w", err)
	}

	r.log.Info("job complete",
		zap.String("repo", repoName),
		zap.String("branch", branch),
		zap.String("commit", commitSHA),
		zap.String("zigzag_version", result.ZigzagVersion),
		zap.Int64("size_bytes", result.SizeBytes),
	)
	return result, nil
}

// collectResult reads report.json for the zigzag version and sums the total
// size of all files in the reports directory. Both operations run concurrently.
func collectResult(reportsDir string) (*Result, error) {
	var (
		version   string
		totalSize atomic.Int64
	)

	g, _ := errgroup.WithContext(context.Background())

	// Parse zigzag version from report.json.
	g.Go(func() error {
		data, err := os.ReadFile(filepath.Join(reportsDir, "report.json"))
		if err != nil {
			return fmt.Errorf("read report.json: %w", err)
		}
		var report struct {
			Meta struct {
				Version string `json:"version"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(data, &report); err != nil {
			return fmt.Errorf("parse report.json: %w", err)
		}
		version = report.Meta.Version
		return nil
	})

	// Walk directory and stat files concurrently.
	g.Go(func() error {
		// Collect file paths first, then stat in parallel.
		var paths []string
		err := filepath.WalkDir(reportsDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			paths = append(paths, path)
			return nil
		})
		if err != nil {
			return fmt.Errorf("walk reports dir: %w", err)
		}

		var wg sync.WaitGroup
		for _, p := range paths {
			wg.Add(1)
			go func(path string) {
				defer wg.Done()
				if info, err := os.Stat(path); err == nil {
					totalSize.Add(info.Size())
				}
			}(p)
		}
		wg.Wait()
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &Result{
		ReportsDir:    reportsDir,
		ZigzagVersion: version,
		SizeBytes:     totalSize.Load(),
	}, nil
}
