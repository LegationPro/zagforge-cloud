package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

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
}

func New(cloner RepoCloner, cfg Config) *Runner {
	return &Runner{cloner: cloner, cfg: cfg}
}

// Dispatch satisfies handler.Dispatcher. It runs the job in a goroutine,
// detached from the HTTP request context so the handler can return immediately.
func (r *Runner) Dispatch(ctx context.Context, event provider.WebhookEvent) {
	go func() {
		if err := r.Run(context.Background(), event); err != nil {
			log.Printf("runner: job failed repo=%s branch=%s commit=%s: %v",
				event.RepoName, event.Branch, event.CommitSHA, err)
		}
	}()
}

// Run executes the full job: generate token → clone → zigzag → cleanup.
func (r *Runner) Run(ctx context.Context, event provider.WebhookEvent) error {
	token, err := r.cloner.GenerateCloneToken(ctx, event.InstallationID)
	if err != nil {
		return fmt.Errorf("generate clone token: %w", err)
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

	cmd := exec.CommandContext(ctx, r.cfg.ZigzagBin, "run", "--output-dir", r.cfg.ReportsDir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("zigzag run: %w: %s", err, out)
	}

	return nil
}
