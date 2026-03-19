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
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/runner"
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
	defer log.Sync()

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

	ch, err := githubprovider.NewClientHandler(client)
	if err != nil {
		return fmt.Errorf("create client handler: %w", err)
	}

	run := runner.New(ch, runner.Config{
		WorkspaceDir: c.Worker.WorkspaceDir,
		ZigzagBin:    c.Worker.ZigzagBin,
		ReportsDir:   c.Worker.ReportsDir,
	}, log)

	svc := service.NewJobService(database, run, log)
	wh := handler.NewWebhookHandler(ch, svc, log)

	health := handler.NewHealthHandler(pool)

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
		return fmt.Errorf("register routes: %w", err)
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
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	log.Info("waiting for in-flight jobs to complete")
	run.Wait()
	log.Info("server stopped")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
