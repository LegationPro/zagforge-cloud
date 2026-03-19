package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LegationPro/zagforge-mvp-impl/shared/go/jobtoken"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/logger"
	githubprovider "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider/github"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/runner"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/storage"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/store"
	"github.com/LegationPro/zagforge-mvp-impl/worker/internal/apiclient"
	"github.com/LegationPro/zagforge-mvp-impl/worker/internal/worker/config"
	"github.com/LegationPro/zagforge-mvp-impl/worker/internal/worker/executor"
	"github.com/LegationPro/zagforge-mvp-impl/worker/internal/worker/poller"
)

const pollInterval = 2 * time.Second

func run() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log, err := logger.New(cfg.AppEnv)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer func() { _ = log.Sync() }()

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to db: %w", err)
	}
	defer pool.Close()

	queries := store.New(pool)

	client, err := githubprovider.NewAPIClient(cfg.GitHub.AppID, cfg.GitHub.PrivateKey, cfg.GitHub.WebhookSecret)
	if err != nil {
		return fmt.Errorf("create API client: %w", err)
	}

	ch, err := githubprovider.NewClientHandler(client, log)
	if err != nil {
		return fmt.Errorf("create client handler: %w", err)
	}

	gcs, err := storage.NewClient(context.Background(), storage.Config{
		Bucket:   cfg.GCS.Bucket,
		Endpoint: cfg.GCS.Endpoint,
	}, log)
	if err != nil {
		return fmt.Errorf("create gcs client: %w", err)
	}

	signer := jobtoken.NewSigner([]byte(cfg.HMACSigningKey), 30*time.Minute)
	api := apiclient.NewClient(cfg.APIBaseURL, signer, log)

	r := runner.New(ch, runner.Config{
		WorkspaceDir: cfg.WorkspaceDir,
		ZigzagBin:    cfg.ZigzagBin,
		ReportsDir:   cfg.ReportsDir,
		JobTimeout:   cfg.JobTimeout,
	}, log)

	exec := executor.NewExecutor(api, gcs, r, log)
	p := poller.NewPoller(queries, r, exec, log, pollInterval, cfg.MaxConcurrency)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return p.Run(ctx)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
