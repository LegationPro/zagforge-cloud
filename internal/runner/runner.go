package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/LegationPro/zagforge-mvp-impl/internal/provider"
)

// RepoCloner is the subset of provider.Worker the runner needs.
type RepoCloner interface {
	GenerateCloneToken(ctx context.Context, installationID int64) (string, error)
	CloneRepo(ctx context.Context, repoURL, ref, token, dst string) error
}

// Config holds runner settings, all resolvable from environment variables via config.LoadWorkerConfig.
type Config struct {
	WorkspaceDir string // base dir for temporary clone directories
	ZigzagBin    string // path to the zigzag binary
	ReportsDir   string // absolute path where zigzag writes reports
}

// Runner clones a repo, runs zigzag, then cleans up the temporary clone.
type Runner struct {
	cloner RepoCloner
	cfg    Config
	wg     sync.WaitGroup
}

func New(cloner RepoCloner, cfg Config) *Runner {
	return &Runner{cloner: cloner, cfg: cfg}
}

// Dispatch satisfies handler.Dispatcher. It runs the job in a goroutine,
// detached from the HTTP request context so the handler can return immediately.
func (r *Runner) Dispatch(ctx context.Context, event provider.WebhookEvent) {
	r.wg.Go(func() {
		defer r.wg.Done()
		if err := r.Run(context.Background(), event); err != nil {
			log.Printf("runner: job failed repo=%s branch=%s commit=%s: %v",
				event.RepoName, event.Branch, event.CommitSHA, err)
		}
	})
}

// Wait blocks until all in-flight jobs complete. Call during graceful shutdown.
func (r *Runner) Wait() {
	r.wg.Wait()
}

// Run executes the full job: generate token → clone → zigzag → cleanup.
func (r *Runner) Run(ctx context.Context, event provider.WebhookEvent) error {
	log.Printf("runner: starting job repo=%s branch=%s commit=%s", event.RepoName, event.Branch, event.CommitSHA)

	token, err := r.cloner.GenerateCloneToken(ctx, event.InstallationID)
	if err != nil {
		return fmt.Errorf("generate clone token: %w", err)
	}

	if err := os.MkdirAll(r.cfg.WorkspaceDir, 0o755); err != nil {
		return fmt.Errorf("create workspace dir: %w", err)
	}

	workDir, err := os.MkdirTemp(r.cfg.WorkspaceDir, "job-*")
	if err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	repoDir := filepath.Join(workDir, "repo")
	if err := r.cloner.CloneRepo(ctx, event.CloneURL, event.Branch, token, repoDir); err != nil {
		return fmt.Errorf("clone repo: %w", err)
	}

	log.Printf("runner: running zigzag repo=%s reports=%s", event.RepoName, r.cfg.ReportsDir)
	cmd := exec.CommandContext(ctx, r.cfg.ZigzagBin, "run", "--no-watch", "--output-dir", r.cfg.ReportsDir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("zigzag run: %w: %s", err, out)
	}

	log.Printf("runner: job complete repo=%s branch=%s commit=%s reports=%s",
		event.RepoName, event.Branch, event.CommitSHA, r.cfg.ReportsDir)
	return nil
}
