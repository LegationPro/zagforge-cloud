package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LegationPro/zagforge-mvp-impl/api/internal/config"
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/db"
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/handler"
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/service"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/logger"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/router"

	githubprovider "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider/github"
	"go.uber.org/zap"
)

func run() error {
	c, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log, err := logger.New(os.Getenv("APP_ENV"))
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer func() { _ = log.Sync() }()

	pool, err := db.Connect(context.Background(), c.DB.URL)
	if err != nil {
		return fmt.Errorf("connect to db: %w", err)
	}
	defer pool.Close()

	database := db.New(pool)

	client, err := githubprovider.NewAPIClient(c.App.GithubAppID, []byte(c.App.GithubAppPrivateKey), c.App.GithubAppWebhookSecret)
	if err != nil {
		return fmt.Errorf("create API client: %w", err)
	}

	ch, err := githubprovider.NewClientHandler(client, log)
	if err != nil {
		return fmt.Errorf("create client handler: %w", err)
	}

	svc := service.NewJobService(database, log)
	wh := handler.NewWebhookHandler(ch, svc, log)
	health := handler.NewHealthHandler(pool)
	api := handler.NewAPIHandler(database, log)

	r := router.New()

	healthRoutes := r.Group()
	if err := healthRoutes.Create([]router.Subroute{
		{Method: router.GET, Path: "/healthz", Handler: health.Liveness},
		{Method: router.GET, Path: "/readyz", Handler: health.Readiness},
	}); err != nil {
		return fmt.Errorf("register health routes: %w", err)
	}

	internal := r.Group()
	if err := internal.Create([]router.Subroute{
		{Method: router.POST, Path: "/internal/webhooks/github", Handler: wh.ServeHTTP},
	}); err != nil {
		return fmt.Errorf("register internal routes: %w", err)
	}

	v1 := r.Group()
	if err := v1.Create([]router.Subroute{
		{Method: router.GET, Path: "/api/v1/repos/{repoID}", Handler: api.GetRepo},
		{Method: router.GET, Path: "/api/v1/repos/{repoID}/jobs", Handler: api.ListJobs},
		{Method: router.GET, Path: "/api/v1/repos/{repoID}/jobs/{jobID}", Handler: api.GetJob},
		{Method: router.GET, Path: "/api/v1/repos/{repoID}/snapshots", Handler: api.ListSnapshots},
		{Method: router.GET, Path: "/api/v1/repos/{repoID}/snapshots/latest", Handler: api.GetLatestSnapshot},
		{Method: router.GET, Path: "/api/v1/snapshots/{snapshotID}", Handler: api.GetSnapshot},
	}); err != nil {
		return fmt.Errorf("register api routes: %w", err)
	}

	srv := &http.Server{
		Addr:    ":" + c.Server.Port,
		Handler: r.Handler(),
	}

	go func() {
		log.Info("server listening", zap.String("port", c.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()

	log.Info("shutting down server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	log.Info("server stopped")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
