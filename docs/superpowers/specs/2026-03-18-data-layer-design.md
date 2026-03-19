# Data Layer Design — 2026-03-18

## Overview

Restructure the current flat single-module Go repo into a three-module monorepo (`api`, `worker`, `shared/go`) bridged by `go.work`, and add the database layer to the `api` service: migrations, sqlc-generated queries, DB connection, and webhook-to-job orchestration with production-grade concurrency safety.

---

## 1. Module Layout

Multi-module monorepo with a root `go.work` bridging three modules:

```
zagforge-mvp-impl/
├── go.work
├── go.work.sum
├── docker-compose.dev.yaml
│
├── api/                           # API service — own go.mod
│   ├── cmd/main.go                # moved from cmd/server/main.go
│   ├── sqlc.yaml                  # points output to internal/db/sqlc/
│   └── internal/
│       ├── config/                # moved from internal/config/
│       ├── handler/               # moved from internal/handler/
│       ├── runner/                # stays here this phase (moved to worker/ when Cloud Tasks ships)
│       ├── service/
│       │   └── job.go             # JobService — dedup + dispatch orchestration
│       └── db/
│           ├── migrations/
│           │   ├── 000001_initial.up.sql
│           │   └── 000001_initial.down.sql
│           ├── queries/
│           │   ├── organizations.sql
│           │   ├── repositories.sql
│           │   ├── jobs.sql
│           │   └── snapshots.sql
│           ├── sqlc/              # generated — never hand-edited
│           │   ├── models.go
│           │   ├── querier.go
│           │   └── *.sql.go
│           ├── sqlc/types.go      # hand-written: JobStatus type + constants (not regenerated)
│           ├── connect.go         # pgxpool setup
│           └── db.go              # DB struct wrapping pool + querier
│
├── worker/                        # Worker service — own go.mod
│   └── cmd/main.go                # moved from cmd/zigzag/main.go (zigzag CLI wrapper only)
│
└── shared/
    └── go/                        # Shared library — own go.mod
        └── provider/
            ├── provider.go        # WebhookEvent, Repo types
            └── github/            # moved from internal/provider/
```

### Phase note: runner stays in `api` this phase

The final architecture has the runner as a separate Cloud Run Job invoked via Cloud Tasks. For this phase (no Cloud Tasks yet), the runner remains in `api/internal/runner/` as an in-process goroutine. When Cloud Tasks is implemented, the runner moves to `worker/internal/runner/`. This avoids a cross-module `api → worker` import dependency that would contradict the microservices boundary.

`worker/` this phase contains only the zigzag CLI wrapper (`cmd/main.go`), giving it its own module boundary without introducing premature coupling.

### Go module paths

| Directory | Module path |
|---|---|
| `api/` | `github.com/LegationPro/zagforge-mvp-impl/api` |
| `worker/` | `github.com/LegationPro/zagforge-mvp-impl/worker` |
| `shared/go/` | `github.com/LegationPro/zagforge-mvp-impl/shared/go` |

`go.work` lists all three. No `replace` directives needed.

Both `api` and `worker` import `shared/go/provider/github`. The `api` uses `ValidateWebhook`; the `worker` (future) uses `GenerateCloneToken` and `CloneRepo`.

---

## 2. Data Layer

### Migrations (`api/internal/db/migrations/`)

`api/internal/db/migrations/` is the canonical location. This differs from `architecture/13-repo-structure.md` (`api/db/migrations/`) — the consolidated `internal/db/` layout was chosen for stronger encapsulation.

Single initial migration (`000001_initial.up.sql`) with all four tables. Down migration drops in reverse order.

**`jobs` table** (full DDL — diverges from `architecture/02-data-model.md` in three ways: adds `superseded` to CHECK, adds `delivery_id`, adds `updated_at`):

```sql
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

-- Keep updated_at current on every update
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
```

`delivery_id` is nullable (not all events have a delivery ID) and is in `CREATE TABLE`. `superseded` is in the CHECK constraint.

**`superseded` vs `cancelled`:** `superseded` means "replaced by a newer push to the same branch." `cancelled` is reserved for explicit user/system intent. Both are terminal states (a superseded job will never run).

**Other tables** (`organizations`, `repositories`, `snapshots`) follow `architecture/02-data-model.md` exactly.

**Key indexes:**

```sql
-- Fast dedup query on every webhook (partial index)
CREATE INDEX idx_jobs_active_branch ON jobs (repo_id, branch)
WHERE status IN ('queued', 'running');

-- Webhook idempotency
CREATE UNIQUE INDEX idx_jobs_delivery_id ON jobs (delivery_id)
WHERE delivery_id IS NOT NULL;

-- Watchdog timeout query
CREATE INDEX idx_jobs_running ON jobs (status, started_at)
WHERE status = 'running';
```

### `JobStatus` type (`api/internal/db/sqlc/types.go`)

Hand-written file inside the `dbsqlc` package, never touched by `sqlc generate`:

```go
package dbsqlc

type JobStatus string

const (
    JobStatusQueued     JobStatus = "queued"
    JobStatusRunning    JobStatus = "running"
    JobStatusSucceeded  JobStatus = "succeeded"
    JobStatusFailed     JobStatus = "failed"
    JobStatusCancelled  JobStatus = "cancelled"
    JobStatusSuperseded JobStatus = "superseded"
)

func (s JobStatus) IsTerminal() bool {
    switch s {
    case JobStatusSucceeded, JobStatusFailed, JobStatusCancelled, JobStatusSuperseded:
        return true
    }
    return false
}
```

`superseded` is terminal — a superseded job will never be dispatched or retried.

### sqlc Queries (`api/internal/db/queries/`)

| File | Queries |
|---|---|
| `organizations.sql` | `UpsertOrg`, `GetOrgByClerkID` |
| `repositories.sql` | `UpsertRepo`, `GetRepoByGithubID` |
| `jobs.sql` | `CreateJob`, `GetActiveJobsForBranch`, `MarkJobSuperseded`, `UpdateJobStatus` |
| `snapshots.sql` | `InsertSnapshot`, `GetLatestSnapshot`, `GetSnapshotsByBranch`, `GetSnapshotByID` |

**`GetRepoByGithubID`** takes `github_repo_id BIGINT` — matches `WebhookEvent.RepoID int64` directly.

**`GetActiveJobsForBranch`** returns all rows where `status IN ('queued', 'running') ORDER BY created_at ASC`. The caller iterates and supersedes only `queued` rows; `running` rows are left untouched.

**`MarkJobSuperseded`** — single UUID, one update per call:

```sql
-- name: MarkJobSuperseded :exec
UPDATE jobs
SET status = 'superseded'
WHERE id = $1;
```

One round-trip per superseded job. Concurrent superseded jobs are rare in practice; a bulk variant is not needed at this scale.

### sqlc.yaml

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

`JobStatus` is declared in the hand-written `internal/db/sqlc/types.go`. The `import` key is omitted because the type lives in the same package as the generated output — adding an import would cause a self-import compile error.

### DB Connection

**`connect.go`:**
```go
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error)
```

**`db.go`:**
```go
package db

type DB struct {
    Pool    *pgxpool.Pool
    Queries *dbsqlc.Queries
}

func New(pool *pgxpool.Pool) *DB {
    return &DB{Pool: pool, Queries: dbsqlc.New(pool)}
}
```

`DB` is created in `cmd/main.go` and injected into `JobService`. No global state.

---

## 3. Docker Compose (Local Dev)

`docker-compose.dev.yaml` at repo root:

- **`postgres`** — `postgres:16-alpine`, port `5432`, named volume `zagforge_pgdata`, healthcheck: `pg_isready -U zagforge`
- **`api`** — builds from `api/Dockerfile.dev` (Air hot reload), `depends_on: postgres: condition: service_healthy`, repo root volume-mounted at `/app` so `go.work` and `go.work.sum` resolve, `DATABASE_URL=postgres://zagforge:zagforge@postgres:5432/zagforge?sslmode=disable`

**`api/Dockerfile.dev`** — single stage:

```dockerfile
FROM golang:1.26-alpine
RUN go install github.com/air-verse/air@latest
WORKDIR /app
# Copy workspace files first for layer caching
COPY go.work go.work.sum ./
COPY api/go.mod api/go.sum ./api/
COPY worker/go.mod worker/go.sum ./worker/
COPY shared/go/go.mod shared/go/go.sum ./shared/go/
RUN go work sync
COPY . .
WORKDIR /app/api
CMD ["air"]
```

`api/.air.toml` — build command: `go build -o /tmp/server ./cmd/`, binary: `/tmp/server`.

**Taskfile targets:**

```yaml
migrate-up:
  cmds:
    - migrate -path api/internal/db/migrations -database $DATABASE_URL up

migrate-down:
  cmds:
    - migrate -path api/internal/db/migrations -database $DATABASE_URL down 1

migrate-create:
  vars:
    NAME: '{{.NAME | default "change"}}'
  cmds:
    - migrate create -ext sql -dir api/internal/db/migrations -seq {{.NAME}}
```

Install golang-migrate: `go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest`

---

## 4. Webhook → Job Orchestration

### Design decision: supersede, not update-in-place

`architecture/03-job-system.md` specifies updating `commit_sha` in place when a `queued` job exists. This spec overrides that: always insert a new `queued` job and mark the old one `superseded`. Reasons: preserves audit history, avoids mutation races, simpler retry logic.

### Service layer

`JobService` owns all business logic. The webhook handler is transport-only: parse event → call service → return 202.

```go
// dispatcher is satisfied by *runner.Runner (in api/internal/runner/ this phase)
type dispatcher interface {
    Dispatch(ctx context.Context, event provider.WebhookEvent)
}

type JobService struct {
    db  *db.DB
    run dispatcher
}

func (s *JobService) HandlePush(ctx context.Context, event provider.WebhookEvent, deliveryID string) error
```

`HandlePush` returns only `error`. The handler returns 500 on error, 202 on success.

`deliveryID` is extracted from the `X-GitHub-Delivery` request header before calling `HandlePush`. If the header is absent, an empty string is passed — stored as NULL in the DB, which the partial unique index handles correctly (NULLs are not considered equal, so multiple NULL rows are permitted).

### HandlePush flow

Org and repo records are created by the GitHub App install flow (future work). If the repo is not yet registered, push events are silently dropped (not an error).

```
handler → JobService.HandlePush(ctx, event, deliveryID)

BEGIN TX

  1. GetRepoByGithubID(event.RepoID)          // int64 → BIGINT
     → if not found: ROLLBACK, return nil     // not registered, drop silently

  2. Acquire advisory lock:
     pg_advisory_xact_lock(hashtext(repo.ID::text || ':' || event.Branch)::bigint)

  3. GetActiveJobsForBranch(repo.ID, event.Branch)   // ORDER BY created_at ASC
     → iterate:
        - IF status == queued  → MarkJobSuperseded(job.ID)
        - IF status == running → leave untouched

  4. CreateJob(repo.ID, event.Branch, event.CommitSHA, deliveryID)
     → on unique constraint violation (delivery_id): ROLLBACK, return nil (duplicate delivery)

COMMIT

5. s.run.Dispatch(context.Background(), event)   // detached context — runner manages its own lifecycle
```

**Advisory lock note:** `hashtext` returns `int4`, widened to `int8` by the cast — valid SQL. `hashtext` is a Postgres-internal function with no cross-version stability guarantee. The collision probability is negligible at this cardinality; the instability risk is accepted for MVP. If cross-version stability is ever required, replace with a CRC32 or FNV-64 hash computed in Go and passed as a literal parameter.

**Dispatch context:** `s.run.Dispatch` receives `context.Background()`, not the request context. This matches the existing `runner.Runner.Dispatch` behavior (which already uses `context.Background()` for the goroutine) and prevents the goroutine from being cancelled when the HTTP handler returns.

**Duplicate delivery handling:** `delivery_id` has a unique partial index. On constraint violation, unwrap the pgx error:

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" {
    return nil // duplicate delivery, treat as no-op
}
return err
```

**Dispatch failure:** if dispatch fails after commit, the job stays `queued`. The watchdog (future work) retries eligible jobs:

```sql
WHERE status = 'queued' AND created_at < now() - interval '2 minutes'
```

---

## 5. What This Does Not Cover (Future Work)

- GitHub App install flow — org + repo record creation via `/auth/github/callback`
- Runner migration to `worker/` module when Cloud Tasks ships
- Runner → DB status updates (`/internal/jobs/start`, `/internal/jobs/complete` callbacks)
- Cloud Tasks integration
- Watchdog / retry loop
- Public API endpoints (`/api/v1/{org}/{repo}/...`)
- Clerk auth middleware

---

## Commit Strategy

1. **Restructure** — move files to new module layout, update imports, add `go.work` + `go.work.sum`, verify `go build ./...` passes in each module
2. **Data layer** — migrations (with trigger), `sqlc/types.go`, `sqlc.yaml`, queries, `connect.go`, `db.go`, run `sqlc generate`
3. **Docker Compose + Taskfile** — local Postgres, migrate targets, `api/Dockerfile.dev`, `api/.air.toml`
4. **Wiring** — `JobService`, dispatcher interface, wire DB into webhook handler, update `api/cmd/main.go`. The handler's success response changes from `200 OK` to `202 Accepted`; the existing webhook handler tests must be updated to assert 202. The handler must extract `X-GitHub-Delivery` and pass it to `HandlePush`.
