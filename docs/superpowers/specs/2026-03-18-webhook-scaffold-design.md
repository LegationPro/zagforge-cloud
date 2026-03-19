# Webhook Scaffold Design

**Date:** 2026-03-18
**Scope:** `execos` module — `POST /internal/webhooks/github` endpoint scaffold

---

## Problem

The execos module has stub provider and handler code but no HTTP server and no real webhook security logic. The goal is a working scaffold that enforces the HMAC-SHA256 signature contract GitHub requires, while keeping downstream processing (job dedup, Cloud Tasks dispatch) explicitly stubbed.

---

## Approach

Option B — Full provider scaffold: implement real HMAC-SHA256 validation inside `ClientHandler.ValidateWebhook`, wire a Chi HTTP server with the route, and leave downstream stubs clearly marked with `// TODO`.

---

## Interface Restructuring (`internal/provider/`)

The existing monolithic `Worker` interface is split. The old four-argument `ValidateWebhook(ctx, payload, signature, secret)` is **replaced** — `secret` moves to construction time. Both `WebhookValidator` and `Worker` use the new three-argument form.

```go
// internal/provider/client.go

type WebhookValidator interface {
    ValidateWebhook(ctx context.Context, payload []byte, signature string) (WebhookEvent, error)
}

type Worker interface {
    WebhookValidator
    GenerateCloneToken(ctx context.Context, installationID int64) (string, error)
    CloneRepo(ctx context.Context, repoURL, ref, token, dst string) error
    ListRepos(ctx context.Context, installationID int64) ([]Repo, error)
}
```

`webhookSecret` is injected at construction. `NewAPIClient` returns an error if `webhookSecret` is empty — an empty secret silently accepts any payload and must be rejected at startup:

```go
type APIClient struct {
    appID         int64
    privateKey    []byte
    webhookSecret string
}

func NewAPIClient(appID int64, privateKey []byte, webhookSecret string) (*APIClient, error) {
    if len(privateKey) == 0 {
        return nil, errors.New("privateKey must not be empty")
    }
    if webhookSecret == "" {
        return nil, errors.New("webhookSecret must not be empty")
    }
    return &APIClient{appID: appID, privateKey: privateKey, webhookSecret: webhookSecret}, nil
}
```

`ClientHandler` compile-time guard remains `var _ Worker = (*ClientHandler)(nil)`. Only `ValidateWebhook` gets real logic; the other three methods remain stubs.

A sentinel error is added to `internal/provider/github.go`:

```go
var ErrInvalidSignature = errors.New("invalid webhook signature")
```

---

## HMAC Validation (`internal/provider/client.go`)

```go
func (h *ClientHandler) ValidateWebhook(ctx context.Context, payload []byte, signature string) (WebhookEvent, error) {
    mac := hmac.New(sha256.New, []byte(h.client.webhookSecret))
    mac.Write(payload)
    expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

    if !hmac.Equal([]byte(expected), []byte(signature)) {
        return WebhookEvent{}, ErrInvalidSignature
    }

    var event WebhookEvent
    // TODO: parse payload JSON into WebhookEvent
    // TODO: read EventType from X-GitHub-Event header (passed in via ctx or as an extra param)
    return event, nil
}
```

All imports (`crypto/hmac`, `crypto/sha256`, `encoding/hex`) are stdlib — no new dependencies beyond Chi.

---

## HTTP Handler (`internal/handler/`)

`internal/handler/handler.go` is deleted. The `Handler` interface with unexported `refreshTokenOnExpiry`/`generateToken` methods does not fit the HTTP handler pattern; token management belongs elsewhere.

`internal/handler/webhook.go` is rewritten. Body size is checked **after** reading: `io.LimitReader` silently truncates at the limit without returning an error, so reading `maxPayloadBytes+1` bytes and checking the actual length is the correct way to detect oversized payloads and return 413 rather than a misleading 401:

```go
const maxPayloadBytes = 25 * 1024 * 1024 // 25 MiB — GitHub's documented maximum webhook payload size

// compile-time guard
var _ http.Handler = (*WebhookHandler)(nil)

type WebhookHandler struct {
    validator provider.WebhookValidator
}

func NewWebhookHandler(v provider.WebhookValidator) *WebhookHandler {
    return &WebhookHandler{validator: v}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    body, err := io.ReadAll(io.LimitReader(r.Body, maxPayloadBytes+1))
    if err != nil {
        http.Error(w, "failed to read body", http.StatusInternalServerError)
        return
    }
    if int64(len(body)) > maxPayloadBytes {
        http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
        return
    }

    signature := r.Header.Get("X-Hub-Signature-256")
    if signature == "" {
        http.Error(w, "missing signature", http.StatusUnauthorized)
        return
    }

    // TODO: pass r.Header.Get("X-GitHub-Event") into ValidateWebhook for event routing
    event, err := h.validator.ValidateWebhook(r.Context(), body, signature)
    if errors.Is(err, provider.ErrInvalidSignature) {
        http.Error(w, "invalid signature", http.StatusUnauthorized)
        return
    }
    if err != nil {
        http.Error(w, "validation error", http.StatusInternalServerError)
        return
    }

    // TODO: dispatch event downstream (job dedup, Cloud Tasks)
    _ = event
    w.WriteHeader(http.StatusOK)
}
```

`WebhookHandler` depends on `provider.WebhookValidator` only — not the full `Worker`.

---

## Server Entrypoint (`cmd/server/main.go`)

New binary. Config from env vars via stdlib `os.Getenv`. `GITHUB_APP_WEBHOOK_SECRET` is validated before construction (empty secret rejected). `go get github.com/go-chi/chi/v5` updates both `go.mod` and `go.sum`:

```go
package main

import (
    "log"
    "net/http"
    "os"
    "strconv"

    "github.com/go-chi/chi/v5"
    "github.com/LegationPro/execos/internal/handler"
    "github.com/LegationPro/execos/internal/provider"
)

func main() {
    appIDStr := os.Getenv("GITHUB_APP_ID")
    appID, err := strconv.ParseInt(appIDStr, 10, 64)
    if err != nil {
        log.Fatalf("invalid GITHUB_APP_ID: %v", err)
    }

    privateKey := []byte(os.Getenv("GITHUB_APP_PRIVATE_KEY"))
    webhookSecret := os.Getenv("GITHUB_APP_WEBHOOK_SECRET")

    client, err := provider.NewAPIClient(appID, privateKey, webhookSecret)
    if err != nil {
        log.Fatalf("failed to create API client: %v", err)
    }
    ch := provider.NewClientHandler(client)
    wh := handler.NewWebhookHandler(ch)

    r := chi.NewRouter()
    r.Post("/internal/webhooks/github", wh.ServeHTTP)

    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    log.Printf("server listening on :%s", port)
    if err := http.ListenAndServe(":"+port, r); err != nil {
        log.Fatalf("server error: %v", err)
    }
}
```

---

## Zigzag launcher (`cmd/zigzag/main.go`)

`cmd/main.go` is moved to `cmd/zigzag/main.go`. The blank import `_ "github.com/LegationPro/execos/internal/provider"` is **removed** — the launcher has no reason to import the provider package. No other code changes.

---

## File Map

| File | Action |
|---|---|
| `cmd/main.go` | Move → `cmd/zigzag/main.go`; remove blank provider import |
| `cmd/server/main.go` | New |
| `internal/provider/github.go` | Add `ErrInvalidSignature`; add `EventType string` field to `WebhookEvent` |
| `internal/provider/client.go` | Replace old four-arg `ValidateWebhook` with three-arg form; add `webhookSecret` to `APIClient`; `NewAPIClient` returns `(*APIClient, error)`; split interfaces; implement `ValidateWebhook` |
| `internal/handler/handler.go` | Delete |
| `internal/handler/webhook.go` | Rewrite as HTTP handler with `var _ http.Handler` guard |
| `go.mod` / `go.sum` | `go get github.com/go-chi/chi/v5` |

---

## Known Gaps (deferred)

- JSON payload parsing into `WebhookEvent` fields — marked `// TODO`
- `X-GitHub-Event` header propagation into `WebhookEvent.EventType` — field added to struct now, population marked `// TODO`
- Job deduplication and Cloud Tasks dispatch — marked `// TODO`
- Graceful shutdown
- Structured logging (uses `log` stdlib for now)
- Config library (`caarlos0/env` deferred to a later phase)
- Tests
- `go.mod` declares `go 1.25.0` — this pre-dates the Go 1.25 release; inherited from existing `go.mod`, not changed by this scaffold
