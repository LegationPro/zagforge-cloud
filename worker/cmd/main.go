package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/LegationPro/zagforge/shared/go/jobtoken"
	"github.com/LegationPro/zagforge/shared/go/logger"
	githubprovider "github.com/LegationPro/zagforge/shared/go/provider/github"
	"github.com/LegationPro/zagforge/shared/go/runner"
	"github.com/LegationPro/zagforge/shared/go/storage"
	"github.com/LegationPro/zagforge/shared/go/store"
	"github.com/LegationPro/zagforge/worker/internal/apiclient"
	"github.com/LegationPro/zagforge/worker/internal/worker/config"
	"github.com/LegationPro/zagforge/worker/internal/worker/executor"
	"github.com/LegationPro/zagforge/worker/internal/worker/handler"
	"github.com/LegationPro/zagforge/worker/internal/worker/poller"
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
	if cfg.HMACSigningKeyPrev != "" {
		signer = signer.WithPreviousKey([]byte(cfg.HMACSigningKeyPrev))
		log.Info("HMAC key rotation active: accepting both current and previous signing keys")
	}
	api := apiclient.NewClient(cfg.APIBaseURL, signer, log)

	r := runner.New(ch, runner.Config{
		WorkspaceDir: cfg.WorkspaceDir,
		ZigzagBin:    cfg.ZigzagBin,
		ReportsDir:   cfg.ReportsDir,
		JobTimeout:   cfg.JobTimeout,
	}, log)

	exec := executor.NewExecutor(api, gcs, r, log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	switch cfg.WorkerMode {
	case "http":
		return runHTTP(ctx, cfg, queries, exec, signer, log)
	case "poll":
		p := poller.NewPoller(queries, r, exec, log, pollInterval, cfg.MaxConcurrency)
		return p.Run(ctx)
	default:
		return fmt.Errorf("unknown WORKER_MODE: %q (expected \"http\" or \"poll\")", cfg.WorkerMode)
	}
}

func runHTTP(ctx context.Context, cfg *config.Config, queries *store.Queries, exec *executor.Executor, signer *jobtoken.Signer, log *zap.Logger) error {
	h := handler.New(queries, exec, signer, log)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /run", h.Run)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	go func() {
		log.Info("worker http server listening", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	<-ctx.Done()

	log.Info("shutting down worker http server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	log.Info("worker http server stopped")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

// test
