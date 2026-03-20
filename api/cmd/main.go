package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/LegationPro/zagforge/api/internal/config"
	"github.com/LegationPro/zagforge/api/internal/db"
	"github.com/LegationPro/zagforge/api/internal/engine"
	apihandler "github.com/LegationPro/zagforge/api/internal/handler/api"
	"github.com/LegationPro/zagforge/api/internal/handler/callback"
	"github.com/LegationPro/zagforge/api/internal/handler/githubauth"
	"github.com/LegationPro/zagforge/api/internal/handler/health"
	"github.com/LegationPro/zagforge/api/internal/handler/watchdog"
	"github.com/LegationPro/zagforge/api/internal/handler/webhook"
	"github.com/LegationPro/zagforge/api/internal/middleware/auth"
	"github.com/LegationPro/zagforge/api/internal/middleware/contenttype"
	corsmw "github.com/LegationPro/zagforge/api/internal/middleware/cors"
	jobtokenmw "github.com/LegationPro/zagforge/api/internal/middleware/jobtoken"
	"github.com/LegationPro/zagforge/api/internal/middleware/ratelimit"
	"github.com/LegationPro/zagforge/api/internal/middleware/watchdogauth"
	"github.com/LegationPro/zagforge/api/internal/service"
	"github.com/LegationPro/zagforge/shared/go/jobtoken"
	"github.com/LegationPro/zagforge/shared/go/logger"
	githubprovider "github.com/LegationPro/zagforge/shared/go/provider/github"
	"github.com/LegationPro/zagforge/shared/go/router"
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

	// Redis for rate limiting.
	redisOpts, err := redis.ParseURL(c.Redis.URL)
	if err != nil {
		return fmt.Errorf("parse redis url: %w", err)
	}
	rdb := redis.NewClient(redisOpts)
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Warn("failed to close redis", zap.Error(err))
		}
	}()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}

	client, err := githubprovider.NewAPIClient(c.App.GithubAppID, []byte(c.App.GithubAppPrivateKey), c.App.GithubAppWebhookSecret)
	if err != nil {
		return fmt.Errorf("create API client: %w", err)
	}

	ch, err := githubprovider.NewClientHandler(client, log)
	if err != nil {
		return fmt.Errorf("create client handler: %w", err)
	}

	clerk.SetKey(c.App.ClerkSecretKey)

	signer := jobtoken.NewSigner([]byte(c.App.HMACSigningKey), 30*time.Minute)
	if c.App.HMACSigningKeyPrev != "" {
		signer = signer.WithPreviousKey([]byte(c.App.HMACSigningKeyPrev))
		log.Info("HMAC key rotation active: accepting both current and previous signing keys")
	}

	// Cloud Tasks enqueuer (or noop for local dev).
	var enqueuer engine.TaskEnqueuer
	if c.CloudTasks.Enabled() {
		ct, err := engine.NewCloudTasksEnqueuer(context.Background(), engine.CloudTasksConfig{
			Project:   c.CloudTasks.Project,
			Location:  c.CloudTasks.Location,
			Queue:     c.CloudTasks.Queue,
			WorkerURL: c.CloudTasks.WorkerURL,
		})
		if err != nil {
			return fmt.Errorf("create cloud tasks enqueuer: %w", err)
		}
		defer func() { _ = ct.Close() }()
		enqueuer = ct
		log.Info("cloud tasks enqueuer enabled",
			zap.String("queue", c.CloudTasks.Queue),
			zap.String("worker_url", c.CloudTasks.WorkerURL),
		)
	} else {
		enqueuer = engine.NewNoopEnqueuer(log)
		log.Info("cloud tasks not configured, using noop enqueuer (poller mode)")
	}

	svc := service.NewJobService(database, log, enqueuer, signer)
	wh := webhook.NewHandler(ch, svc, log)
	healthH := health.NewHandler(pool)
	apiH := apihandler.NewHandler(database, log)
	callbackH := callback.NewHandler(database, ch, log)
	watchdogH := watchdog.NewHandler(database, log)
	githubAuthH := githubauth.NewHandler(database, c.App.GithubAppSlug, log)

	r := router.New()
	r.Use(corsmw.Cors(c.CORS.AllowedOrigins))

	// Health — no auth, no rate limit.
	healthRoutes := r.Group()
	if err := healthRoutes.Create([]router.Subroute{
		{Method: router.GET, Path: "/healthz", Handler: healthH.Liveness},
		{Method: router.GET, Path: "/readyz", Handler: healthH.Readiness},
	}); err != nil {
		return fmt.Errorf("register health routes: %w", err)
	}

	// GitHub App OAuth — no auth, redirects to GitHub and handles callback.
	authRoutes := r.Group()
	if err := authRoutes.Create([]router.Subroute{
		{Method: router.GET, Path: "/auth/github/install", Handler: githubAuthH.Install},
		{Method: router.GET, Path: "/auth/github/callback", Handler: githubAuthH.Callback},
	}); err != nil {
		return fmt.Errorf("register github auth routes: %w", err)
	}

	// Webhooks — Content-Type + rate limited by IP, higher burst (GitHub sends bursts).
	internal := r.Group()
	internal.Use(contenttype.RequireJSON())
	internal.Use(ratelimit.RateLimit(rdb, ratelimit.RateLimitConfig{
		MaxRequests: 120,
		Window:      1 * time.Minute,
	}, "webhook", log))
	if err := internal.Create([]router.Subroute{
		{Method: router.POST, Path: "/internal/webhooks/github", Handler: wh.ServeHTTP},
	}); err != nil {
		return fmt.Errorf("register internal routes: %w", err)
	}

	// Job callbacks — Content-Type + signed job token auth.
	callbacks := r.Group()
	callbacks.Use(contenttype.RequireJSON())
	callbacks.Use(jobtokenmw.Auth(signer, log))
	if err := callbacks.Create([]router.Subroute{
		{Method: router.POST, Path: "/internal/jobs/start", Handler: callbackH.Start},
		{Method: router.POST, Path: "/internal/jobs/complete", Handler: callbackH.Complete},
	}); err != nil {
		return fmt.Errorf("register callback routes: %w", err)
	}

	// Watchdog — shared secret auth (Cloud Scheduler in production uses OIDC).
	watchdogRoutes := r.Group()
	watchdogRoutes.Use(watchdogauth.SharedSecret(c.App.WatchdogSecret))
	if err := watchdogRoutes.Create([]router.Subroute{
		{Method: router.POST, Path: "/internal/watchdog/timeout", Handler: watchdogH.Timeout},
	}); err != nil {
		return fmt.Errorf("register watchdog routes: %w", err)
	}

	// API v1 — auth first (rejects unauthenticated), then rate limit by user ID.
	v1 := r.Group()
	v1.Use(auth.Auth(log))
	v1.Use(ratelimit.RateLimit(rdb, ratelimit.RateLimitConfig{
		MaxRequests: 60,
		Window:      1 * time.Minute,
	}, "api", log))
	if err := v1.Create([]router.Subroute{
		{Method: router.GET, Path: "/api/v1/repos/{repoID}", Handler: apiH.GetRepo},
		{Method: router.GET, Path: "/api/v1/repos/{repoID}/jobs", Handler: apiH.ListJobs},
		{Method: router.GET, Path: "/api/v1/repos/{repoID}/jobs/{jobID}", Handler: apiH.GetJob},
		{Method: router.GET, Path: "/api/v1/repos/{repoID}/snapshots", Handler: apiH.ListSnapshots},
		{Method: router.GET, Path: "/api/v1/repos/{repoID}/snapshots/latest", Handler: apiH.GetLatestSnapshot},
		{Method: router.GET, Path: "/api/v1/snapshots/{snapshotID}", Handler: apiH.GetSnapshot},
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
