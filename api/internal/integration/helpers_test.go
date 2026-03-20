//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	dbpkg "github.com/LegationPro/zagforge/api/internal/db"
	apihandler "github.com/LegationPro/zagforge/api/internal/handler/api"
	"github.com/LegationPro/zagforge/api/internal/handler/health"
	"github.com/LegationPro/zagforge/api/internal/handler/watchdog"
	corsmw "github.com/LegationPro/zagforge/api/internal/middleware/cors"
	"github.com/LegationPro/zagforge/shared/go/httputil"
	"github.com/LegationPro/zagforge/shared/go/router"
	"github.com/LegationPro/zagforge/shared/go/store"
)

type testEnv struct {
	server *httptest.Server
	db     *dbpkg.DB
	pool   *pgxpool.Pool
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://zagforge:zagforge@localhost:5432/zagforge_test?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to test db: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test db: %v", err)
	}

	database := dbpkg.New(pool)
	log := zap.NewNop()

	r := router.New()
	r.Use(corsmw.Cors([]string{"http://localhost:3000"}))

	healthH := health.NewHandler(pool)
	apiH := apihandler.NewHandler(database, log)
	watchdogH := watchdog.NewHandler(database, log)

	healthRoutes := r.Group()
	_ = healthRoutes.Create([]router.Subroute{
		{Method: router.GET, Path: "/healthz", Handler: healthH.Liveness},
		{Method: router.GET, Path: "/readyz", Handler: healthH.Readiness},
	})

	watchdogRoutes := r.Group()
	_ = watchdogRoutes.Create([]router.Subroute{
		{Method: router.POST, Path: "/internal/watchdog/timeout", Handler: watchdogH.Timeout},
	})

	v1 := r.Group()
	_ = v1.Create([]router.Subroute{
		{Method: router.GET, Path: "/api/v1/repos/{repoID}", Handler: apiH.GetRepo},
		{Method: router.GET, Path: "/api/v1/repos/{repoID}/jobs", Handler: apiH.ListJobs},
		{Method: router.GET, Path: "/api/v1/repos/{repoID}/jobs/{jobID}", Handler: apiH.GetJob},
		{Method: router.GET, Path: "/api/v1/repos/{repoID}/snapshots", Handler: apiH.ListSnapshots},
		{Method: router.GET, Path: "/api/v1/repos/{repoID}/snapshots/latest", Handler: apiH.GetLatestSnapshot},
		{Method: router.GET, Path: "/api/v1/snapshots/{snapshotID}", Handler: apiH.GetSnapshot},
	})

	server := httptest.NewServer(r.Handler())

	env := &testEnv{server: server, db: database, pool: pool}

	t.Cleanup(func() {
		server.Close()
		pool.Close()
	})

	return env
}

func (e *testEnv) seed(t *testing.T) (orgID, repoID string) {
	t.Helper()
	ctx := context.Background()

	org, err := e.db.Queries.UpsertOrg(ctx, store.UpsertOrgParams{
		ClerkOrgID: fmt.Sprintf("test_org_%d", time.Now().UnixNano()),
		Slug:       fmt.Sprintf("test-%d", time.Now().UnixNano()),
		Name:       "Test Org",
	})
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}

	repo, err := e.db.Queries.UpsertRepo(ctx, store.UpsertRepoParams{
		OrgID:          org.ID,
		GithubRepoID:   time.Now().UnixNano(),
		InstallationID: 12345,
		FullName:       fmt.Sprintf("test-org/test-repo-%d", time.Now().UnixNano()),
		DefaultBranch:  "main",
	})
	if err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	return org.ID.String(), repo.ID.String()
}

func (e *testEnv) createJob(t *testing.T, repoID, branch, commitSHA string) string {
	t.Helper()
	ctx := context.Background()

	repoUUID, err := httputil.UUIDFromString(repoID)
	if err != nil {
		t.Fatalf("parse repo id: %v", err)
	}

	job, err := e.db.Queries.CreateJob(ctx, store.CreateJobParams{
		RepoID:    repoUUID,
		Branch:    branch,
		CommitSha: commitSHA,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	return job.ID.String()
}

func (e *testEnv) get(t *testing.T, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(e.server.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}
