# What's Next — Phase 3 & 4 Implementation Guide

This doc maps out everything left to implement. Phase 1 (project setup, data layer) and Phase 2 (core loop) are done. What remains is infrastructure-as-code (Terraform), production networking, deploy pipelines, and operational tooling.

---

## Current State (done)

Everything below is already built and working on the `dev` branch (30+ commits ahead of `main`):

- Three-module monorepo: `api/`, `worker/`, `shared/go/` with `go.work`
- DB layer: migrations, sqlc, pgx connection pool
- Webhook handler with HMAC validation + job dedup (advisory locks)
- Worker: poller, executor, handler, API client callbacks
- Cloud Tasks integration with dual-key rotation
- GCS storage for snapshots
- Clerk auth middleware, job token middleware (HMAC signed)
- Rate limiting middleware (Redis)
- Watchdog endpoint (timeout stale jobs)
- API handlers: repos, jobs, snapshots
- Internal callback handlers: `/internal/jobs/start`, `/internal/jobs/complete`
- CI pipeline: lint, test, Docker build (`ci.yml`)
- Docker build & push to GHCR (`docker.yml`)
- Docker Compose local dev with infra (Postgres, Redis, fake-GCS)
- Production Dockerfiles for api, worker, migrate

---

## Phase 3: Infrastructure & Terraform

### 3.1 Terraform Module Structure

Create the full `terraform/` directory. No Terraform exists yet.

**Read:** [architecture/11-terraform.md](architecture/11-terraform.md) — full module tree, state management, env tfvars

Files to create:
```
terraform/
  main.tf, variables.tf, outputs.tf, backend.tf
  envs/  dev.tfvars, staging.tfvars, prod.tfvars
  modules/
    networking/   — LB, Cloud Armor, SSL cert, DNS
    database/     — Neon (dev) / Cloud SQL (prod)
    redis/        — Memorystore instance
    storage/      — GCS bucket, IAM, lifecycle
    api/          — Cloud Run service, IAM, env vars, secrets
    worker/       — Cloud Run Job, IAM, env vars, secrets
    queue/        — Cloud Tasks queue config
    scheduler/    — Cloud Scheduler (watchdog cron)
    registry/     — Artifact Registry repo
    secrets/      — Secret Manager secrets + IAM bindings
```

Key decisions from the spec:
- Remote state in GCS bucket `zagforge-terraform-state`
- `lifecycle { ignore_changes = [image] }` so Terraform doesn't fight with deploys over the image tag
- Cloud Armor is conditional (`cloud_armor_enabled` variable) — off in dev, on in prod
- Neon Postgres for dev (free), Cloud SQL for prod

### 3.2 Cloud Load Balancer + Cloud Armor

**Read:** [architecture/08-networking.md](architecture/08-networking.md) — LB routing rules, Cloud Armor WAF, rate limiting layers

What to implement:
- Global external Application Load Balancer (L7)
- Path-prefix routing: `/api/v1/*`, `/auth/*`, `/internal/webhooks/*`, `/internal/jobs/*`, `/internal/watchdog/*` all go to the `api` Cloud Run service
- Managed SSL certificate for `api.zagforge.com`
- Cloud Armor security policy:
  - DDoS: adaptive protection, 1000 req/IP/min default
  - WAF: OWASP ModSecurity CRS (SQLi, XSS)
  - Body size limit: 10MB
  - Internal endpoint IP restrictions where possible
- Health check on `/healthz`

**Note:** The architecture says LB + Cloud Armor are optional during dev (saves ~$23/month). Can be added when custom domain is needed.

### 3.3 Secret Manager

**Read:** [architecture/05-authentication.md](architecture/05-authentication.md) — secrets list, rotation strategy

Secrets to provision:
| Secret | Rotation |
|---|---|
| GitHub App private key | Manual |
| GitHub App webhook secret | Manual |
| HMAC signing key (job tokens) | Quarterly, dual-version |
| Redis auth password | With instance recreation |
| DATABASE_URL | Per-environment |
| CLERK_SECRET_KEY | Manual |

---

## Phase 4: CI/CD & Production Deploys

### 4.1 Deploy Pipelines (GitHub Actions)

**Read:** [architecture/10-cicd.md](architecture/10-cicd.md) — full workflow YAML for CI, deploy-api, deploy-worker

What's already done:
- `ci.yml` — lint, test, build (exists, working)
- `docker.yml` — build & push to GHCR on merge to main (exists, working)

What's missing:
- **`deploy-api.yml`** — triggers on push to `main` (paths: `api/**`, `shared/**`). Builds image, pushes to Artifact Registry, deploys to Cloud Run via `gcloud run deploy`
- **`deploy-worker.yml`** — same pattern but for `worker/` Cloud Run Job
- Workload Identity Federation (WIF) auth — no service account keys in GitHub
- Integration test job with Postgres service container
- `sqlc diff` check (ensures generated code is up to date)

**Note:** The current `docker.yml` pushes to GHCR. The architecture spec targets GCP Artifact Registry. Decide whether to keep GHCR or migrate to Artifact Registry.

### 4.2 Makefile — Manual Deploys & Operations

**Read:** [architecture/14-deployment-ops.md](architecture/14-deployment-ops.md) — full Makefile, rollback playbook, canary deploys

No root `Makefile` exists yet. Create it with these targets:

| Target | What it does |
|---|---|
| `image-push` | Build + push image tagged with git SHA |
| `deploy` | Deploy a specific commit to Cloud Run |
| `revisions` | List recent revisions (pick one for rollback) |
| `rollback` | Shift 100% traffic to a previous revision |
| `canary` | Route N% traffic to latest revision |
| `status` | Show current traffic split |
| `logs` | Show recent Cloud Run logs |
| `compose-dev` | Start local dev env via Doppler |
| `compose-dev-d` | Same but detached |
| `stop` | Stop all compose services |
| `test-integration` | Spin up dev compose + run integration tests |

Also create per-service Makefiles:

**Read:** [architecture/12-local-dev.md](architecture/12-local-dev.md) — `api/Makefile` (build, migrate-*, sqlc), `worker/Makefile` (build, run, test)

### 4.3 Production Docker Compose

**Read:** [architecture/12-local-dev.md](architecture/12-local-dev.md) — `docker-compose.yaml` (prod-like, no volume mounts)

A `docker-compose.yaml` for production-like local builds (no Air, no volume mounts, built images) is specced but may not exist yet. Check `docker/` directory for current state.

### 4.4 Doppler Integration

**Read:** [architecture/12-local-dev.md](architecture/12-local-dev.md) — Doppler project structure, onboarding flow, `.env.example`

What to set up:
- Doppler project `zagforge` with `dev`, `staging`, `prod` configs
- `.env.example` committed as documentation (no real secrets)
- Compose commands wrapped in `doppler run --`
- Migration commands wrapped in `doppler run --`

---

## Additional Items (from architecture, not yet implemented)

### GitHub App OAuth Flow

**Read:** [architecture/04-api-endpoints.md](architecture/04-api-endpoints.md)

Two endpoints not yet built:
- `GET /auth/github/install` — redirect to GitHub App installation
- `GET /auth/github/callback` — handle installation callback

### CORS Headers

**Read:** [architecture/05-authentication.md](architecture/05-authentication.md) — CORS section

Public API should serve:
```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, OPTIONS
Access-Control-Allow-Headers: Authorization, Content-Type
```

### Integration Tests

**Read:** [architecture/10-cicd.md](architecture/10-cicd.md) — `test-integration` job

The CI spec includes a `test-integration` job that spins up docker-compose, waits for `/healthz`, and runs `go test -tags=integration`. No integration test files exist yet.

---

## Suggested Order

1. **Merge `dev` into `main`** — 30+ commits sitting on dev
2. **Root Makefile + per-service Makefiles** — immediate developer QoL
3. **Terraform scaffold** — `terraform/` with all modules stubbed out
4. **Terraform: secrets, registry, storage, database** — foundational infra
5. **Terraform: api, worker, queue, scheduler** — Cloud Run services
6. **Terraform: networking** — LB + Cloud Armor (can defer if no custom domain yet)
7. **Deploy pipelines** — `deploy-api.yml`, `deploy-worker.yml`
8. **GitHub App OAuth flow** — `/auth/github/install`, `/auth/github/callback`
9. **CORS middleware**
10. **Integration tests**

---

## Architecture Docs Quick Reference

| Doc | Phase | What's in it |
|---|---|---|
| [01-overview.md](architecture/01-overview.md) | All | Tech stack, phases, system diagram |
| [02-data-model.md](architecture/02-data-model.md) | 1 | Tables: organizations, repositories, jobs, snapshots |
| [03-job-system.md](architecture/03-job-system.md) | 2 | Job state machine, dedup, watchdog, Cloud Tasks config |
| [04-api-endpoints.md](architecture/04-api-endpoints.md) | 2 | All public + internal + auth endpoints |
| [05-authentication.md](architecture/05-authentication.md) | 2 | Auth mechanisms, job tokens, CORS, config, secrets |
| [06-provider-and-worker.md](architecture/06-provider-and-worker.md) | 2 | GitHub client, consumer interfaces, worker container |
| [07-storage.md](architecture/07-storage.md) | 2 | GCS layout, snapshot JSON format |
| [08-networking.md](architecture/08-networking.md) | 3 | LB, Cloud Armor, rate limiting layers |
| [09-docker.md](architecture/09-docker.md) | 1 | Dockerfiles (dev + prod) for api and worker |
| [10-cicd.md](architecture/10-cicd.md) | 4 | CI + deploy GitHub Actions workflows |
| [11-terraform.md](architecture/11-terraform.md) | 3 | Full Terraform module structure + example HCL |
| [12-local-dev.md](architecture/12-local-dev.md) | 1 | go.work, Doppler, compose, Makefiles, running locally |
| [13-repo-structure.md](architecture/13-repo-structure.md) | 1 | Full directory tree |
| [14-deployment-ops.md](architecture/14-deployment-ops.md) | 4 | Makefile ops, rollback playbook, canary, staging-to-prod |
