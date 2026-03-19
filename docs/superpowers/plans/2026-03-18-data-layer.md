# Data Layer — Multi-Module Monorepo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure the flat Go module into a three-module monorepo (api, worker, shared/go) bridged by go.work, add a Postgres data layer to the api service, and wire webhook events to a dedup-safe job creation flow.

**Architecture:** Three modules share a root `go.work`. `shared/go` owns provider types and the GitHub client. `api` owns the HTTP server, DB layer (pgx/sqlc/golang-migrate), and the runner (temporary, moves to `worker` when Cloud Tasks ships). `worker` owns only the zigzag CLI wrapper for now. Job deduplication uses a Postgres advisory lock + supersede-on-insert pattern.

**Tech Stack:** Go 1.26, pgx/v5, sqlc, golang-migrate CLI, Chi, Docker Compose, Postgres 16-alpine, Air (hot reload)

---

## File Map

```
zagforge-mvp-impl/
├── go.work                                    CREATE
├── docker-compose.dev.yaml                    CREATE
├── Taskfile.yml                               MODIFY (build paths + migrate targets)
│
├── shared/
│   └── go/
│       ├── go.mod                             CREATE
│       └── provider/
│           ├── provider.go                    CREATE (types + interfaces from internal/provider/github.go)
│           └── github/
│               └── github.go                  CREATE (impl from internal/provider/client.go)
│
├── api/
│   ├── go.mod                                 CREATE
│   ├── sqlc.yaml                              CREATE
│   ├── Dockerfile.dev                         CREATE
│   ├── .air.toml                              CREATE
│   ├── cmd/
│   │   └── main.go                            CREATE (from cmd/server/main.go, updated imports + DB wiring)
│   └── internal/
│       ├── config/
│       │   ├── config.go                      MOVE + add DBConfig
│       │   ├── app.go                         MOVE (import path change only)
│       │   ├── server.go                      MOVE (import path change only)
│       │   ├── worker.go                      MOVE (import path change only)
│       │   └── *_test.go                      MOVE (import path change only)
│       ├── handler/
│       │   ├── webhook.go                     MOVE + replace Dispatcher with pushHandler, 200→202
│       │   └── webhook_test.go                MOVE + update mocks + assert 202
│       ├── runner/
│       │   ├── runner.go                      MOVE (import path change only)
│       │   └── runner_test.go                 MOVE (import path change only)
│       ├── service/
│       │   └── job.go                         CREATE (JobService + HandlePush)
│       └── db/
│           ├── migrations/
│           │   ├── 000001_initial.up.sql      CREATE
│           │   └── 000001_initial.down.sql    CREATE
│           ├── queries/
│           │   ├── organizations.sql          CREATE
│           │   ├── repositories.sql           CREATE
│           │   ├── jobs.sql                   CREATE
│           │   └── snapshots.sql              CREATE
│           ├── sqlc/
│           │   └── types.go                   CREATE (hand-written JobStatus, never regenerated)
│           ├── connect.go                     CREATE
│           └── db.go                          CREATE
│
└── worker/
    ├── go.mod                                 CREATE
    └── cmd/
        └── main.go                            MOVE from cmd/zigzag/main.go
```

**Delete after moving:**
- `internal/` (entire directory)
- `cmd/server/`
- `cmd/zigzag/`

---

## Task 1: Scaffold the three-module structure

**Goal:** Create go.mod files, go.work, and the shared/go provider package. At the end of this task `go build ./...` passes in each module independently.

**Files:**
- Create: `shared/go/go.mod`
- Create: `shared/go/provider/provider.go`
- Create: `shared/go/provider/github/github.go`
- Create: `api/go.mod`
- Create: `worker/go.mod`
- Create: `go.work`

- [ ] **Step 1: Create shared/go module**

```bash
mkdir -p shared/go/provider/github
cd shared/go
go mod init github.com/LegationPro/zagforge-mvp-impl/shared/go
go get github.com/golang-jwt/jwt/v5
```

- [ ] **Step 2: Create `shared/go/provider/provider.go`**

This file contains the types and interfaces currently in `internal/provider/github.go`. Copy that file's contents into `shared/go/provider/provider.go`, change the package declaration to `package provider`, and keep all types as-is. The file should contain: `ErrInvalidSignature`, `ActionType`, `WebhookEvent`, `Repo`, `pushPayload`, `parsePayload`, `branchFromRef`, `buildAuthURL`, `WebhookValidator`, `Worker`.

```go
package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var ErrInvalidSignature = errors.New("invalid webhook signature")

type ActionType string

type WebhookEvent struct {
	EventType      string
	Action         ActionType
	RepoID         int64
	RepoName       string
	CloneURL       string
	Branch         string
	CommitSHA      string
	InstallationID int64
}

type Repo struct {
	ID            int64
	FullName      string
	DefaultBranch string
}

type pushPayload struct {
	Ref    string `json:"ref"`
	After  string `json:"after"`
	Action string `json:"action"`
	Repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

func parsePayload(payload []byte) (pushPayload, error) {
	var p pushPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return pushPayload{}, err
	}
	return p, nil
}

func branchFromRef(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

func buildAuthURL(repoURL, token string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("invalid repo URL: %w", err)
	}
	if token != "" && u.Scheme == "https" {
		u.User = url.UserPassword("x-access-token", token)
	}
	return u.String(), nil
}

type WebhookValidator interface {
	ValidateWebhook(ctx context.Context, payload []byte, signature string, eventType string) (WebhookEvent, error)
}

type Worker interface {
	WebhookValidator
	GenerateCloneToken(ctx context.Context, installationID int64) (string, error)
	CloneRepo(ctx context.Context, repoURL, ref, token, dst string) error
	ListRepos(ctx context.Context, installationID int64) ([]Repo, error)
}
```

Add the missing `"context"` import.

- [ ] **Step 3: Create `shared/go/provider/github/github.go`**

This file contains the GitHub API implementation currently in `internal/provider/client.go`. Copy it into `shared/go/provider/github/github.go`, change the package to `package github`, and update the import of provider types to use the new module path:

```go
package github

import (
	"context"
	// ... (keep existing stdlib imports)
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/provider"
	"github.com/golang-jwt/jwt/v5"
)
```

All references to `WebhookEvent`, `Repo`, `ErrInvalidSignature`, `ActionType`, `pushPayload`, `parsePayload`, `branchFromRef`, `buildAuthURL`, `Worker`, `WebhookValidator` must be prefixed with `provider.`. The `APIClient`, `ClientHandler`, `NewAPIClient`, `NewClientHandler` and all methods stay in this package.

The compile-time guard changes to:
```go
var _ provider.Worker = (*ClientHandler)(nil)
```

- [ ] **Step 4: Verify shared/go builds**

```bash
cd shared/go && go build ./...
```

Expected: no errors.

- [ ] **Step 5: Create api/go.mod**

```bash
mkdir -p api/cmd api/internal/config api/internal/handler api/internal/runner api/internal/service api/internal/db/migrations api/internal/db/queries api/internal/db/sqlc
cd api
go mod init github.com/LegationPro/zagforge-mvp-impl/api
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5
go get github.com/joho/godotenv
go get github.com/LegationPro/zagforge-mvp-impl/shared/go@v0.0.0
```

Note: the `shared/go` require will be overridden by go.work — the version doesn't matter.

- [ ] **Step 6: Create worker/go.mod**

```bash
mkdir -p worker/cmd
cd worker
go mod init github.com/LegationPro/zagforge-mvp-impl/worker
```

- [ ] **Step 7: Create go.work at repo root**

```bash
cd /path/to/zagforge-mvp-impl   # repo root
go work init ./api ./worker ./shared/go
go work sync
```

`go.work` will look like:

```
go 1.26.0

use (
	./api
	./worker
	./shared/go
)
```

- [ ] **Step 8: Commit scaffold**

```bash
git add go.work go.work.sum api/go.mod api/go.sum worker/go.mod shared/go/go.mod shared/go/go.sum shared/go/provider/
git commit -m "chore: scaffold three-module monorepo with shared/go provider"
```

---

## Task 2: Migrate existing packages to api/ and worker/

**Goal:** Move `internal/config`, `internal/handler`, `internal/runner` to `api/internal/`, move `cmd/server` to `api/cmd/`, move `cmd/zigzag` to `worker/cmd/`. Update all import paths. All existing tests pass.

**Files:**
- Create: `api/internal/config/*.go` (from `internal/config/`)
- Create: `api/internal/handler/webhook.go` (from `internal/handler/webhook.go`)
- Create: `api/internal/handler/webhook_test.go` (from `internal/handler/webhook_test.go`)
- Create: `api/internal/runner/runner.go` (from `internal/runner/runner.go`)
- Create: `api/internal/runner/runner_test.go` (from `internal/runner/runner_test.go`)
- Create: `api/cmd/main.go` (from `cmd/server/main.go`)
- Create: `worker/cmd/main.go` (from `cmd/zigzag/main.go`)
- Delete: `internal/`, `cmd/server/`, `cmd/zigzag/`

**Import path changes (old → new):**

| Old | New |
|---|---|
| `github.com/LegationPro/zagforge-mvp-impl/internal/provider` | `github.com/LegationPro/zagforge-mvp-impl/shared/go/provider` |
| `github.com/LegationPro/zagforge-mvp-impl/internal/config` | `github.com/LegationPro/zagforge-mvp-impl/api/internal/config` |
| `github.com/LegationPro/zagforge-mvp-impl/internal/handler` | `github.com/LegationPro/zagforge-mvp-impl/api/internal/handler` |
| `github.com/LegationPro/zagforge-mvp-impl/internal/runner` | `github.com/LegationPro/zagforge-mvp-impl/api/internal/runner` |

- [ ] **Step 1: Copy config package**

Copy all files from `internal/config/` to `api/internal/config/`. No import changes needed — this package only imports stdlib and `github.com/joho/godotenv`. The package declaration stays `package config`.

- [ ] **Step 2: Add DBConfig to api/internal/config/config.go**

Add a `DBConfig` struct and load `DATABASE_URL` in `Load()`:

```go
type DBConfig struct {
	URL string
}

type Config struct {
	App    *AppConfig
	Server *ServerConfig
	Worker *WorkerConfig
	DB     *DBConfig
}
```

In `Load()`, after loading worker config:

```go
dbURL := os.Getenv("DATABASE_URL")
if dbURL == "" {
    return nil, notSetErr("DATABASE_URL")
}
return &Config{App: app, Server: server, Worker: worker, DB: &DBConfig{URL: dbURL}}, nil
```

- [ ] **Step 3: Copy handler package**

Copy `internal/handler/webhook.go` and `internal/handler/webhook_test.go` to `api/internal/handler/`. Update the import:

```go
// webhook.go
import (
    // ...
    "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider"
)
```

```go
// webhook_test.go
import (
    // ...
    "github.com/LegationPro/zagforge-mvp-impl/api/internal/handler"
    "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider"
)
```

- [ ] **Step 4: Copy runner package**

Copy `internal/runner/runner.go` and `internal/runner/runner_test.go` to `api/internal/runner/`. Update the import:

```go
// runner.go
import (
    // ...
    "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider"
)
```

```go
// runner_test.go
import (
    // ...
    "github.com/LegationPro/zagforge-mvp-impl/api/internal/runner"
    "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider"
)
```

- [ ] **Step 5: Create api/cmd/main.go**

Copy `cmd/server/main.go` to `api/cmd/main.go`. Update imports:

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/LegationPro/zagforge-mvp-impl/api/internal/config"
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/handler"
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/runner"
	githubprovider "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider/github"
	"github.com/go-chi/chi/v5"
)
```

Replace `provider.NewAPIClient(...)` and `provider.NewClientHandler(...)` with `githubprovider.NewAPIClient(...)` and `githubprovider.NewClientHandler(...)`.

The `DB` field on config is unused in this commit — do not wire it yet. Leave a `// TODO: wire DB` comment after the config load.

- [ ] **Step 6: Create worker/cmd/main.go**

Copy `cmd/zigzag/main.go` to `worker/cmd/main.go`. No import changes needed (uses only stdlib).

- [ ] **Step 7: Run tests across all modules**

```bash
cd api && go test ./...
cd ../shared/go && go test ./...
cd ../worker && go build ./cmd/
```

All tests should pass. Fix any import errors if found.

- [ ] **Step 8: Delete old directories**

```bash
rm -rf internal/ cmd/server/ cmd/zigzag/
```

- [ ] **Step 9: Verify workspace builds clean**

```bash
# from repo root
go build ./api/cmd/
go build ./worker/cmd/
go test ./...
```

Expected: all green.

- [ ] **Step 10: Update Taskfile.yml**

Replace the existing Taskfile.yml with updated paths and new targets:

```yaml
version: "3"

tasks:
    build:
        desc: Run tests and build all binaries
        cmds:
            - task: test
            - task: build:server
            - task: build:zigzag

    build:server:
        desc: Build the server binary
        cmds:
            - go build -o .bin/server ./api/cmd/

    build:zigzag:
        desc: Build the zigzag wrapper binary
        cmds:
            - go build -o .bin/zigzag ./worker/cmd/

    test:
        desc: Run all tests with race detection
        cmds:
            - go test -v -race -cover ./...

    run:server:dev:
        desc: Build and run the server in dev mode. Pass ENV_FILE=.env
        vars:
            ENV_FILE: '{{.ENV_FILE | default ".env"}}'
        cmds:
            - task: build:server
            - APP_ENV=dev ENV_FILE={{.ENV_FILE}} ./.bin/server

    webhook:forward:
        desc: Forward GitHub webhooks to local server via smee.io
        cmds:
            - smee --url https://smee.io/StaYxmPtfVR1Vq1 --target http://localhost:8080/internal/webhooks/github

    db:up:
        desc: Start local Postgres
        cmds:
            - docker compose -f docker-compose.dev.yaml up -d postgres

    db:down:
        desc: Stop local Postgres
        cmds:
            - docker compose -f docker-compose.dev.yaml down

    migrate-up:
        desc: Run all pending migrations (requires DATABASE_URL env var)
        cmds:
            - migrate -path api/internal/db/migrations -database $DATABASE_URL up

    migrate-down:
        desc: Roll back the last migration (requires DATABASE_URL env var)
        cmds:
            - migrate -path api/internal/db/migrations -database $DATABASE_URL down 1

    migrate-create:
        desc: Create a new migration file. Pass NAME=description
        vars:
            NAME: '{{.NAME | default "change"}}'
        cmds:
            - migrate create -ext sql -dir api/internal/db/migrations -seq {{.NAME}}
```

- [ ] **Step 11: Commit restructure**

```bash
git add -A
git commit -m "chore: restructure to three-module monorepo (api, worker, shared/go)"
```

---

## Task 3: Write migrations and sqlc setup

**Goal:** Define the full DB schema, write sqlc queries, generate Go DB code. No Postgres required for this task — sqlc generates purely from SQL files.

**Prerequisites:** Install tools:
```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

**Files:**
- Create: `api/internal/db/migrations/000001_initial.up.sql`
- Create: `api/internal/db/migrations/000001_initial.down.sql`
- Create: `api/internal/db/sqlc/types.go` (hand-written, never regenerated)
- Create: `api/sqlc.yaml`
- Create: `api/internal/db/queries/organizations.sql`
- Create: `api/internal/db/queries/repositories.sql`
- Create: `api/internal/db/queries/jobs.sql`
- Create: `api/internal/db/queries/snapshots.sql`
- Create: `api/internal/db/sqlc/*.sql.go` (generated)

- [ ] **Step 1: Write the up migration**

Create `api/internal/db/migrations/000001_initial.up.sql`:

```sql
CREATE TABLE organizations (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    clerk_org_id TEXT UNIQUE NOT NULL,
    slug         TEXT UNIQUE NOT NULL,
    name         TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_organizations_slug ON organizations (slug);

CREATE TABLE repositories (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    github_repo_id  BIGINT UNIQUE NOT NULL,
    installation_id BIGINT NOT NULL,
    full_name       TEXT NOT NULL,
    default_branch  TEXT NOT NULL DEFAULT 'main',
    installed_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_repositories_org_id ON repositories (org_id);
CREATE INDEX idx_repositories_full_name ON repositories (full_name);

CREATE TABLE jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id         UUID NOT NULL REFERENCES repositories(id),
    branch          TEXT NOT NULL,
    commit_sha      TEXT NOT NULL,
    delivery_id     TEXT,
    status          TEXT NOT NULL DEFAULT 'queued'
                    CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled', 'superseded')),
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ
);

CREATE INDEX idx_jobs_active_branch ON jobs (repo_id, branch)
    WHERE status IN ('queued', 'running');

CREATE UNIQUE INDEX idx_jobs_delivery_id ON jobs (delivery_id)
    WHERE delivery_id IS NOT NULL;

CREATE INDEX idx_jobs_running ON jobs (status, started_at)
    WHERE status = 'running';

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER jobs_set_updated_at
BEFORE UPDATE ON jobs
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE snapshots (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id          UUID NOT NULL REFERENCES repositories(id),
    job_id           UUID NOT NULL REFERENCES jobs(id),
    branch           TEXT NOT NULL,
    commit_sha       TEXT NOT NULL,
    gcs_path         TEXT NOT NULL,
    snapshot_version INT NOT NULL DEFAULT 1,
    zigzag_version   TEXT NOT NULL,
    size_bytes       BIGINT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_snapshots_latest ON snapshots (repo_id, branch, created_at DESC);
CREATE INDEX idx_snapshots_job_id ON snapshots (job_id);
CREATE UNIQUE INDEX idx_snapshots_unique ON snapshots (repo_id, branch, commit_sha);
```

- [ ] **Step 2: Write the down migration**

Create `api/internal/db/migrations/000001_initial.down.sql`:

```sql
DROP TRIGGER IF EXISTS jobs_set_updated_at ON jobs;
DROP FUNCTION IF EXISTS set_updated_at();
DROP TABLE IF EXISTS snapshots;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS repositories;
DROP TABLE IF EXISTS organizations;
```

- [ ] **Step 3: Create the hand-written JobStatus type**

Create `api/internal/db/sqlc/types.go`. This file must exist **before** running `sqlc generate`. It will never be overwritten by sqlc.

```go
package dbsqlc

// JobStatus is the typed representation of the jobs.status column.
// This file is hand-written and is never touched by sqlc generate.
type JobStatus string

const (
	JobStatusQueued     JobStatus = "queued"
	JobStatusRunning    JobStatus = "running"
	JobStatusSucceeded  JobStatus = "succeeded"
	JobStatusFailed     JobStatus = "failed"
	JobStatusCancelled  JobStatus = "cancelled"
	JobStatusSuperseded JobStatus = "superseded"
)

// IsTerminal returns true if the job is in a final state and will never run again.
func (s JobStatus) IsTerminal() bool {
	switch s {
	case JobStatusSucceeded, JobStatusFailed, JobStatusCancelled, JobStatusSuperseded:
		return true
	}
	return false
}
```

- [ ] **Step 4: Create sqlc.yaml**

Create `api/sqlc.yaml`:

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "internal/db/queries"
    schema: "internal/db/migrations"
    gen:
      go:
        package: "dbsqlc"
        out: "internal/db/sqlc"
        overrides:
          - db_type: "text"
            column: "jobs.status"
            go_type:
              type: "JobStatus"
```

The `import` key is intentionally omitted from `go_type` — `JobStatus` lives in the same package as the generated output, and a self-import would cause a compile error.

- [ ] **Step 5: Write organizations.sql**

Create `api/internal/db/queries/organizations.sql`:

```sql
-- name: UpsertOrg :one
INSERT INTO organizations (clerk_org_id, slug, name)
VALUES ($1, $2, $3)
ON CONFLICT (clerk_org_id) DO UPDATE
    SET name = EXCLUDED.name
RETURNING *;

-- name: GetOrgByClerkID :one
SELECT * FROM organizations WHERE clerk_org_id = $1;
```

- [ ] **Step 6: Write repositories.sql**

Create `api/internal/db/queries/repositories.sql`:

```sql
-- name: UpsertRepo :one
INSERT INTO repositories (org_id, github_repo_id, installation_id, full_name, default_branch)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (github_repo_id) DO UPDATE
    SET installation_id = EXCLUDED.installation_id,
        full_name       = EXCLUDED.full_name
RETURNING *;

-- name: GetRepoByGithubID :one
SELECT * FROM repositories WHERE github_repo_id = $1;
```

- [ ] **Step 7: Write jobs.sql**

Create `api/internal/db/queries/jobs.sql`:

```sql
-- name: CreateJob :one
INSERT INTO jobs (repo_id, branch, commit_sha, delivery_id, status)
VALUES ($1, $2, $3, NULLIF($4, ''), 'queued')
RETURNING *;

-- name: GetActiveJobsForBranch :many
SELECT * FROM jobs
WHERE repo_id = $1
  AND branch = $2
  AND status IN ('queued', 'running')
ORDER BY created_at ASC;

-- name: MarkJobSuperseded :exec
UPDATE jobs
SET status = 'superseded'
WHERE id = $1;

-- name: UpdateJobStatus :exec
UPDATE jobs
SET status       = $2,
    error_message = $3,
    started_at   = $4,
    finished_at  = $5
WHERE id = $1;
```

- [ ] **Step 8: Write snapshots.sql**

Create `api/internal/db/queries/snapshots.sql`:

```sql
-- name: InsertSnapshot :one
INSERT INTO snapshots (repo_id, job_id, branch, commit_sha, gcs_path, snapshot_version, zigzag_version, size_bytes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (repo_id, branch, commit_sha) DO NOTHING
RETURNING *;

-- name: GetLatestSnapshot :one
SELECT * FROM snapshots
WHERE repo_id = $1
  AND branch  = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: GetSnapshotsByBranch :many
SELECT * FROM snapshots
WHERE repo_id = $1
  AND branch  = $2
ORDER BY created_at DESC;

-- name: GetSnapshotByID :one
SELECT * FROM snapshots WHERE id = $1;
```

- [ ] **Step 9: Run sqlc generate**

```bash
cd api && sqlc generate
```

Expected: files created in `api/internal/db/sqlc/` — `db.go`, `models.go`, `querier.go`, `organizations.sql.go`, `repositories.sql.go`, `jobs.sql.go`, `snapshots.sql.go`. Verify `types.go` was not overwritten.

If sqlc errors about unknown types, verify `types.go` exists in `api/internal/db/sqlc/` before running.

- [ ] **Step 10: Verify generated code compiles**

```bash
cd api && go build ./internal/db/...
```

Expected: no errors.

- [ ] **Step 11: Write connect.go and db.go**

Create `api/internal/db/connect.go`:

```go
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect creates and validates a pgxpool connection.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}
```

Create `api/internal/db/db.go`:

```go
package db

import (
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/db/sqlc"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a connection pool and sqlc-generated queries together.
type DB struct {
	Pool    *pgxpool.Pool
	Queries *dbsqlc.Queries
}

// New creates a DB from an existing pool.
func New(pool *pgxpool.Pool) *DB {
	return &DB{Pool: pool, Queries: dbsqlc.New(pool)}
}
```

- [ ] **Step 12: Verify db package builds**

```bash
cd api && go build ./internal/db/...
```

Expected: no errors.

- [ ] **Step 13: Commit data layer**

```bash
git add api/internal/db/ api/sqlc.yaml
git commit -m "feat: add DB migrations, sqlc queries, and connection layer"
```

---

## Task 4: Docker Compose and local dev setup

**Goal:** Local Postgres runs via `task db:up`, migrations apply via `task migrate-up`, and the api service can start with hot reload via `docker compose -f docker-compose.dev.yaml up`.

**Files:**
- Create: `docker-compose.dev.yaml`
- Create: `api/Dockerfile.dev`
- Create: `api/.air.toml`

- [ ] **Step 1: Create docker-compose.dev.yaml**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: zagforge
      POSTGRES_USER: zagforge
      POSTGRES_PASSWORD: zagforge
    ports:
      - "5432:5432"
    volumes:
      - zagforge_pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U zagforge"]
      interval: 5s
      timeout: 5s
      retries: 5

  api:
    build:
      context: .
      dockerfile: api/Dockerfile.dev
    ports:
      - "8080:8080"
    volumes:
      - .:/app
    environment:
      APP_ENV: dev
      PORT: "8080"
      DATABASE_URL: postgres://zagforge:zagforge@postgres:5432/zagforge?sslmode=disable
      GITHUB_APP_ID: ${GITHUB_APP_ID}
      GITHUB_APP_PRIVATE_KEY: ${GITHUB_APP_PRIVATE_KEY}
      GITHUB_APP_WEBHOOK_SECRET: ${GITHUB_APP_WEBHOOK_SECRET}
    depends_on:
      postgres:
        condition: service_healthy

volumes:
  zagforge_pgdata:
```

- [ ] **Step 2: Create api/Dockerfile.dev**

```dockerfile
FROM golang:1.26-alpine
RUN go install github.com/air-verse/air@latest
WORKDIR /app
# Copy workspace manifests first for layer caching
COPY go.work go.work.sum ./
COPY api/go.mod api/go.sum ./api/
COPY worker/go.mod worker/go.sum ./worker/
COPY shared/go/go.mod shared/go/go.sum ./shared/go/
RUN go work sync
COPY . .
WORKDIR /app/api
CMD ["air"]
```

- [ ] **Step 3: Create api/.air.toml**

```toml
root = "."
tmp_dir = "/tmp"

[build]
cmd = "go build -o /tmp/server ./cmd/"
bin = "/tmp/server"
include_ext = ["go"]
exclude_dir = ["internal/db/sqlc"]

[log]
time = true
```

- [ ] **Step 4: Smoke test local Postgres**

```bash
task db:up
sleep 3
DATABASE_URL="postgres://zagforge:zagforge@localhost:5432/zagforge?sslmode=disable" task migrate-up
```

Expected output ends with: `1/u 000001_initial (Xms)`

- [ ] **Step 5: Test migrate-down and back up**

```bash
DATABASE_URL="postgres://zagforge:zagforge@localhost:5432/zagforge?sslmode=disable" task migrate-down
DATABASE_URL="postgres://zagforge:zagforge@localhost:5432/zagforge?sslmode=disable" task migrate-up
```

Expected: clean round-trip with no errors.

- [ ] **Step 6: Commit Docker + Taskfile**

```bash
git add docker-compose.dev.yaml api/Dockerfile.dev api/.air.toml Taskfile.yml
git commit -m "feat: add Docker Compose local dev setup and migrate Taskfile targets"
```

---

## Task 5: JobService and webhook handler wiring

**Goal:** `JobService.HandlePush` implements the dedup flow; the webhook handler delegates to it; `api/cmd/main.go` wires everything together. All existing tests pass; new tests cover the updated handler contract.

**Files:**
- Create: `api/internal/service/job.go`
- Modify: `api/internal/handler/webhook.go`
- Modify: `api/internal/handler/webhook_test.go`
- Modify: `api/cmd/main.go`

- [ ] **Step 1: Write failing handler tests first (TDD)**

In `api/internal/handler/webhook_test.go`, replace `mockDispatcher` with `mockPushHandler` and update the test assertions for `202`. Also add a test for `HandlePush` returning an error.

The current test `TestServeHTTP_validSignature_returns200` becomes `TestServeHTTP_pushEvent_returns202`. The `TestServeHTTP_pushEvent_dispatches` test becomes `TestServeHTTP_pushEvent_callsHandlePush`.

Replace the `mockDispatcher` type and all its usages:

```go
// mockPushHandler replaces mockDispatcher.
type mockPushHandler struct {
	err        error
	called     bool
	deliveryID string
	event      provider.WebhookEvent
}

func (m *mockPushHandler) HandlePush(_ context.Context, event provider.WebhookEvent, deliveryID string) error {
	m.called = true
	m.event = event
	m.deliveryID = deliveryID
	return m.err
}
```

Update `newHandler` to accept `*mockPushHandler` instead of `*mockDispatcher`:

```go
func newHandler(v *mockValidator, svc *mockPushHandler) *handler.WebhookHandler {
	return handler.NewWebhookHandler(v, svc)
}
```

Update/replace tests:

```go
func TestServeHTTP_pushEvent_returns202(t *testing.T) {
	svc := &mockPushHandler{}
	h := newHandler(&mockValidator{event: provider.WebhookEvent{EventType: "push"}}, svc)
	w := post(t, h, []byte(`{}`), "sha256=valid", "push")
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
}

func TestServeHTTP_pushEvent_callsHandlePush(t *testing.T) {
	event := provider.WebhookEvent{EventType: "push", RepoName: "org/repo", Branch: "main"}
	svc := &mockPushHandler{}
	h := newHandler(&mockValidator{event: event}, svc)
	post(t, h, []byte(`{}`), "sha256=valid", "push")
	if !svc.called {
		t.Fatal("expected HandlePush to be called")
	}
	if svc.event.RepoName != "org/repo" {
		t.Errorf("expected RepoName %q, got %q", "org/repo", svc.event.RepoName)
	}
}

func TestServeHTTP_pushEvent_passesDeliveryID(t *testing.T) {
	svc := &mockPushHandler{}
	h := newHandler(&mockValidator{event: provider.WebhookEvent{EventType: "push"}}, svc)
	r := httptest.NewRequest(http.MethodPost, "/internal/webhooks/github", bytes.NewReader([]byte(`{}`)))
	r.Header.Set("X-Hub-Signature-256", "sha256=valid")
	r.Header.Set("X-GitHub-Event", "push")
	r.Header.Set("X-GitHub-Delivery", "abc-123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if svc.deliveryID != "abc-123" {
		t.Errorf("expected deliveryID %q, got %q", "abc-123", svc.deliveryID)
	}
}

func TestServeHTTP_handlePushError_returns500(t *testing.T) {
	svc := &mockPushHandler{err: errors.New("db error")}
	h := newHandler(&mockValidator{event: provider.WebhookEvent{EventType: "push"}}, svc)
	w := post(t, h, []byte(`{}`), "sha256=valid", "push")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// Unsupported events still return 200 (not 202) — acknowledged but not acted on.
// Keep the existing test body unchanged; only the helper signature changes (mockDispatcher → mockPushHandler).
func TestServeHTTP_unsupportedEvent_returns200(t *testing.T) {
	svc := &mockPushHandler{}
	h := newHandler(&mockValidator{event: provider.WebhookEvent{EventType: "ping"}}, svc)
	w := post(t, h, []byte(`{}`), "sha256=valid", "ping")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for unsupported event, got %d", w.Code)
	}
	if svc.called {
		t.Error("expected HandlePush not to be called for unsupported event")
	}
}
```

- [ ] **Step 2: Run tests — confirm they fail**

```bash
cd api && go test ./internal/handler/...
```

Expected: compile error (mockDispatcher not found, handler signature mismatch). This confirms the tests are driving the implementation.

- [ ] **Step 3: Update webhook.go**

Replace `Dispatcher` with `pushHandler` interface and update `ServeHTTP`:

```go
package handler

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/LegationPro/zagforge-mvp-impl/shared/go/provider"
)

const maxPayloadBytes = 25 * 1024 * 1024

var _ http.Handler = (*WebhookHandler)(nil)

var supportedEvents = map[string]bool{
	"push": true,
}

// pushHandler receives a validated push event and delivery ID.
// JobService satisfies this interface.
type pushHandler interface {
	HandlePush(ctx context.Context, event provider.WebhookEvent, deliveryID string) error
}

type WebhookHandler struct {
	validator provider.WebhookValidator
	svc       pushHandler
}

func NewWebhookHandler(v provider.WebhookValidator, svc pushHandler) *WebhookHandler {
	return &WebhookHandler{validator: v, svc: svc}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		http.Error(w, "missing signature", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxPayloadBytes+1))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}
	if int64(len(body)) > maxPayloadBytes {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	event, err := h.validator.ValidateWebhook(r.Context(), body, signature, eventType)
	if errors.Is(err, provider.ErrInvalidSignature) {
		log.Printf("webhook: invalid signature event=%s", eventType)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	if err != nil {
		log.Printf("webhook: validation error event=%s: %v", eventType, err)
		http.Error(w, "validation error", http.StatusInternalServerError)
		return
	}

	if !supportedEvents[event.EventType] {
		log.Printf("webhook: ignoring unsupported event=%s", event.EventType)
		w.WriteHeader(http.StatusOK)
		return
	}

	deliveryID := r.Header.Get("X-GitHub-Delivery")
	log.Printf("webhook: dispatching event=%s repo=%s branch=%s commit=%s",
		event.EventType, event.RepoName, event.Branch, event.CommitSHA)

	if err := h.svc.HandlePush(r.Context(), event, deliveryID); err != nil {
		log.Printf("webhook: handle push error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
```

- [ ] **Step 4: Run handler tests — confirm they pass**

```bash
cd api && go test ./internal/handler/...
```

Expected: all pass.

- [ ] **Step 5: Create api/internal/service/job.go**

```go
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	dbpkg "github.com/LegationPro/zagforge-mvp-impl/api/internal/db"
	dbsqlc "github.com/LegationPro/zagforge-mvp-impl/api/internal/db/sqlc"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/provider"
)

// dispatcher is satisfied by *runner.Runner.
type dispatcher interface {
	Dispatch(ctx context.Context, event provider.WebhookEvent)
}

// JobService orchestrates job creation with deduplication.
// It satisfies handler.pushHandler.
type JobService struct {
	db  *dbpkg.DB
	run dispatcher
}

func NewJobService(db *dbpkg.DB, run dispatcher) *JobService {
	return &JobService{db: db, run: run}
}

// HandlePush persists a new queued job for the push event (with dedup) then dispatches it.
// If the repo is not registered, the event is silently dropped.
// If deliveryID is empty (header absent), it is stored as NULL.
func (s *JobService) HandlePush(ctx context.Context, event provider.WebhookEvent, deliveryID string) error {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := dbsqlc.New(tx)

	// 1. Look up registered repo — drop silently if not found.
	repo, err := qtx.GetRepoByGithubID(ctx, event.RepoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("get repo: %w", err)
	}

	// 2. Acquire per-(repo, branch) advisory lock for the duration of this transaction.
	// hashtext returns int4, cast to int8 for pg_advisory_xact_lock.
	// Collision probability is negligible at this cardinality.
	if _, err := tx.Exec(ctx,
		"SELECT pg_advisory_xact_lock(hashtext($1::text || ':' || $2)::bigint)",
		repo.ID, event.Branch,
	); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}

	// 3. Supersede any existing queued jobs for this branch.
	active, err := qtx.GetActiveJobsForBranch(ctx, dbsqlc.GetActiveJobsForBranchParams{
		RepoID: repo.ID,
		Branch: event.Branch,
	})
	if err != nil {
		return fmt.Errorf("get active jobs: %w", err)
	}
	for _, j := range active {
		if j.Status == dbsqlc.JobStatusQueued {
			if err := qtx.MarkJobSuperseded(ctx, j.ID); err != nil {
				return fmt.Errorf("mark job superseded: %w", err)
			}
		}
	}

	// 4. Insert new queued job. NULLIF converts empty deliveryID to NULL.
	if _, err := qtx.CreateJob(ctx, dbsqlc.CreateJobParams{
		RepoID:    repo.ID,
		Branch:    event.Branch,
		CommitSha: event.CommitSHA,
		DeliveryID: deliveryID,
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil // duplicate delivery, no-op
		}
		return fmt.Errorf("create job: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	// 5. Dispatch outside the transaction.
	s.run.Dispatch(context.Background(), event)
	return nil
}
```

Note: `CreateJobParams.DeliveryID` field type depends on what sqlc generates for the nullable `TEXT` column. If sqlc generates `pgtype.Text`, adjust accordingly:
```go
DeliveryID: pgtype.Text{String: deliveryID, Valid: deliveryID != ""},
```
Check `api/internal/db/sqlc/jobs.sql.go` after generation to confirm the exact type.

- [ ] **Step 6: Verify service package compiles**

```bash
cd api && go build ./internal/service/...
```

Expected: no errors.

- [ ] **Step 7: Update api/cmd/main.go to wire DB and JobService**

Replace the `// TODO: wire DB` placeholder and add DB connection + JobService construction. The final `api/cmd/main.go`:

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/LegationPro/zagforge-mvp-impl/api/internal/config"
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/db"
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/handler"
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/runner"
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/service"
	githubprovider "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider/github"
	"github.com/go-chi/chi/v5"
)

func main() {
	c, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	pool, err := db.Connect(context.Background(), c.DB.URL)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	database := db.New(pool)

	client, err := githubprovider.NewAPIClient(c.App.GithubAppID, []byte(c.App.GithubAppPrivateKey), c.App.GithubAppWebhookSecret)
	if err != nil {
		log.Fatalf("failed to create API client: %v", err)
	}

	ch := githubprovider.NewClientHandler(client)
	run := runner.New(ch, runner.Config{
		WorkspaceDir: c.Worker.WorkspaceDir,
		ZigzagBin:    c.Worker.ZigzagBin,
		ReportsDir:   c.Worker.ReportsDir,
	})

	svc := service.NewJobService(database, run)
	wh := handler.NewWebhookHandler(ch, svc)

	mux := chi.NewRouter()
	mux.Post("/internal/webhooks/github", wh.ServeHTTP)

	srv := &http.Server{
		Addr:    ":" + c.Server.Port,
		Handler: mux,
	}

	go func() {
		log.Printf("server listening on :%s", c.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()

	log.Println("shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown failed: %v", err)
	}

	log.Println("waiting for in-flight jobs to complete...")
	run.Wait()
	log.Println("server stopped")
}
```

- [ ] **Step 8: Run all tests**

```bash
cd api && go test -race ./...
```

Expected: all pass. The `service` package has no unit tests yet (integration tests require Postgres and are deferred). Handler, config, and runner tests all pass.

- [ ] **Step 9: Build the server binary end-to-end**

```bash
task build:server
```

Expected: `.bin/server` produced, no errors.

- [ ] **Step 10: Smoke test with local Postgres**

Ensure Postgres is running and migrations are applied (from Task 4). Start the server:

```bash
DATABASE_URL="postgres://zagforge:zagforge@localhost:5432/zagforge?sslmode=disable" \
APP_ENV=dev ENV_FILE=.env task run:server:dev
```

Expected: server starts, connects to DB, listens on :8080. A push webhook for an unregistered repo should return 202 with no errors logged (silent drop path).

- [ ] **Step 11: Commit wiring**

```bash
git add api/internal/service/ api/internal/handler/ api/cmd/main.go
git commit -m "feat: wire JobService into webhook handler with dedup job creation"
```
