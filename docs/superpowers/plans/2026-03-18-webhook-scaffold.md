# Webhook Scaffold Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `POST /internal/webhooks/github` HTTP endpoint to execos that validates GitHub HMAC-SHA256 signatures, with downstream dispatch stubbed for later implementation.

**Architecture:** A new `cmd/server/main.go` binary wires a Chi router to a `WebhookHandler`. The handler reads the request body, enforces a 25 MiB size limit, and delegates signature validation to a `WebhookValidator` interface satisfied by `ClientHandler`. The existing zigzag launcher moves to `cmd/zigzag/main.go` unchanged.

**Tech Stack:** Go 1.25, `github.com/go-chi/chi/v5`, stdlib `crypto/hmac`, `crypto/sha256`, `net/http`, `net/http/httptest` (tests)

---

## File Map

| File | Status | Responsibility |
|---|---|---|
| `cmd/zigzag/main.go` | Move from `cmd/main.go` | Zigzag binary launcher (no logic change) |
| `cmd/server/main.go` | Create | HTTP server entrypoint — wires config, provider, handler, router |
| `internal/provider/github.go` | Modify | `WebhookEvent` types + sentinel errors |
| `internal/provider/client.go` | Modify | Interfaces, `APIClient` construction, `ValidateWebhook` implementation |
| `internal/provider/client_test.go` | Create | Tests for `NewAPIClient` validation and `ValidateWebhook` |
| `internal/handler/handler.go` | Delete | Old `Handler` interface — replaced by `http.Handler` pattern |
| `internal/handler/webhook.go` | Rewrite | `WebhookHandler` — HTTP body reading, size limit, signature dispatch |
| `internal/handler/webhook_test.go` | Create | Tests for `ServeHTTP` — all HTTP-level cases |

---

## Task 1: Relocate zigzag launcher

**Files:**
- Create: `cmd/zigzag/main.go`
- Delete: `cmd/main.go`

- [ ] **Step 1: Create the new directory and move the file**

```bash
mkdir -p cmd/zigzag
cp cmd/main.go cmd/zigzag/main.go
```

- [ ] **Step 2: Remove the blank provider import from `cmd/zigzag/main.go`**

Open `cmd/zigzag/main.go`. Remove this line — the launcher has no reason to import the provider:

```go
_ "github.com/LegationPro/execos/internal/provider"
```

Also remove `"github.com/LegationPro/execos/internal/provider"` from the import block entirely (it should be the only import to remove).

- [ ] **Step 3: Delete the original**

```bash
rm cmd/main.go
```

- [ ] **Step 4: Verify compilation**

```bash
go build ./cmd/zigzag/...
```

Expected: exits 0, no output.

---

## Task 2: Add Chi dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Fetch Chi**

```bash
go get github.com/go-chi/chi/v5
```

Expected output includes a line like:
```
go: added github.com/go-chi/chi/v5 v5.x.x
```

- [ ] **Step 2: Verify go.mod contains the new require**

Open `go.mod` and confirm a `require` entry for `github.com/go-chi/chi/v5` is present.

---

## Task 3: Update provider types (`internal/provider/github.go`)

**Files:**
- Modify: `internal/provider/github.go`

- [ ] **Step 1: Add `EventType` field and sentinel error**

Open `internal/provider/github.go`. Make it look exactly like this:

```go
package provider

import "errors"

// ErrInvalidSignature is returned by ValidateWebhook when the HMAC signature does not match.
var ErrInvalidSignature = errors.New("invalid webhook signature")

type ActionType string

// WebhookEvent is the parsed result of a validated webhook payload.
// Validation would not be necessary on these fields since they're returned from GitHub already.
type WebhookEvent struct {
	EventType string // value of X-GitHub-Event header; populated in a future task
	Action    ActionType
	RepoID    int64
	RepoName  string
	Branch    string
	CommitSHA string
}

type Repo struct {
	ID            int64
	FullName      string
	DefaultBranch string
}
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/provider/...
```

Expected: exits 0, no output.

---

## Task 4: Refactor provider client (TDD)

**Files:**
- Create: `internal/provider/client_test.go`
- Modify: `internal/provider/client.go`

Tests cover: constructor validation (empty secret, empty key), valid signature acceptance, invalid signature rejection.

- [ ] **Step 1: Write the failing tests**

Create `internal/provider/client_test.go`:

```go
package provider_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/LegationPro/execos/internal/provider"
)

func validClient(t *testing.T) *provider.ClientHandler {
	t.Helper()
	client, err := provider.NewAPIClient(1, []byte("private-key"), "webhook-secret")
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	return provider.NewClientHandler(client)
}

func makeSignature(t *testing.T, secret string, payload []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestNewAPIClient_rejectsEmptyWebhookSecret(t *testing.T) {
	_, err := provider.NewAPIClient(1, []byte("key"), "")
	if err == nil {
		t.Fatal("expected error for empty webhookSecret, got nil")
	}
}

func TestNewAPIClient_rejectsEmptyPrivateKey(t *testing.T) {
	_, err := provider.NewAPIClient(1, nil, "secret")
	if err == nil {
		t.Fatal("expected error for nil privateKey, got nil")
	}
}

func TestNewAPIClient_rejectsEmptyPrivateKeySlice(t *testing.T) {
	_, err := provider.NewAPIClient(1, []byte{}, "secret")
	if err == nil {
		t.Fatal("expected error for empty privateKey slice, got nil")
	}
}

func TestNewAPIClient_succeedsWithValidInputs(t *testing.T) {
	_, err := provider.NewAPIClient(1, []byte("key"), "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateWebhook_validSignature(t *testing.T) {
	ch := validClient(t)
	payload := []byte(`{"action":"push"}`)
	sig := makeSignature(t, "webhook-secret", payload)

	_, err := ch.ValidateWebhook(context.Background(), payload, sig)
	if err != nil {
		t.Fatalf("expected no error for valid signature, got: %v", err)
	}
}

func TestValidateWebhook_invalidSignature(t *testing.T) {
	ch := validClient(t)
	payload := []byte(`{"action":"push"}`)

	_, err := ch.ValidateWebhook(context.Background(), payload, "sha256=badhex")
	if !errors.Is(err, provider.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got: %v", err)
	}
}

func TestValidateWebhook_wrongSecret(t *testing.T) {
	ch := validClient(t)
	payload := []byte(`{"action":"push"}`)
	sig := makeSignature(t, "wrong-secret", payload)

	_, err := ch.ValidateWebhook(context.Background(), payload, sig)
	if !errors.Is(err, provider.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for wrong secret, got: %v", err)
	}
}

func TestValidateWebhook_emptySignature(t *testing.T) {
	ch := validClient(t)

	_, err := ch.ValidateWebhook(context.Background(), []byte("payload"), "")
	if !errors.Is(err, provider.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for empty signature, got: %v", err)
	}
}
```

- [ ] **Step 2: Run tests — expect compile failure**

```bash
go test ./internal/provider/...
```

Expected: compile error — `NewAPIClient` currently returns `*APIClient`, not `(*APIClient, error)`.

- [ ] **Step 3: Rewrite `internal/provider/client.go`**

Replace the entire file:

```go
package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// WebhookValidator is the minimal interface required by consumers that only validate webhooks.
type WebhookValidator interface {
	ValidateWebhook(ctx context.Context, payload []byte, signature string) (WebhookEvent, error)
}

// Worker is the full provider interface. It embeds WebhookValidator and adds the
// operations needed by the worker container (clone, upload).
type Worker interface {
	WebhookValidator
	GenerateCloneToken(ctx context.Context, installationID int64) (string, error)
	CloneRepo(ctx context.Context, repoURL, ref, token, dst string) error
	ListRepos(ctx context.Context, installationID int64) ([]Repo, error)
}

// APIClient holds provider credentials. Construct with NewAPIClient.
type APIClient struct {
	appID         int64
	privateKey    []byte
	webhookSecret string
}

// NewAPIClient returns a configured APIClient. Returns an error if privateKey or
// webhookSecret is empty — both are required for correct operation at startup.
func NewAPIClient(appID int64, privateKey []byte, webhookSecret string) (*APIClient, error) {
	if len(privateKey) == 0 {
		return nil, errors.New("privateKey must not be empty")
	}
	if webhookSecret == "" {
		return nil, errors.New("webhookSecret must not be empty")
	}
	return &APIClient{
		appID:         appID,
		privateKey:    privateKey,
		webhookSecret: webhookSecret,
	}, nil
}

// ClientHandler wraps an APIClient and satisfies the Worker interface.
type ClientHandler struct {
	client *APIClient
}

// Compile-time guard: ClientHandler must satisfy Worker.
var _ Worker = (*ClientHandler)(nil)

func NewClientHandler(client *APIClient) *ClientHandler {
	return &ClientHandler{client: client}
}

// ValidateWebhook validates the HMAC-SHA256 signature of a GitHub webhook payload.
// The signature must be in the format "sha256=<hex>" as sent by GitHub in
// the X-Hub-Signature-256 header. Uses constant-time comparison to prevent timing attacks.
func (h *ClientHandler) ValidateWebhook(ctx context.Context, payload []byte, signature string) (WebhookEvent, error) {
	mac := hmac.New(sha256.New, []byte(h.client.webhookSecret))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return WebhookEvent{}, ErrInvalidSignature
	}

	var event WebhookEvent
	// TODO: parse payload JSON into WebhookEvent fields
	// TODO: populate event.EventType from X-GitHub-Event header (pass via ctx or extra param)
	return event, nil
}

func (h *ClientHandler) GenerateCloneToken(ctx context.Context, installationID int64) (string, error) {
	return "", nil // TODO
}

func (h *ClientHandler) CloneRepo(ctx context.Context, repoURL, ref, token, dst string) error {
	return nil // TODO
}

func (h *ClientHandler) ListRepos(ctx context.Context, installationID int64) ([]Repo, error) {
	return nil, nil // TODO
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./internal/provider/... -v
```

Expected: all 8 tests pass.

---

## Task 5: Rewrite webhook handler (TDD)

**Files:**
- Delete: `internal/handler/handler.go`
- Rewrite: `internal/handler/webhook.go`
- Create: `internal/handler/webhook_test.go`

The handler tests use a `mockValidator` to stay isolated from the provider implementation.

- [ ] **Step 1: Write the failing tests**

Create `internal/handler/webhook_test.go`:

```go
package handler_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LegationPro/execos/internal/handler"
	"github.com/LegationPro/execos/internal/provider"
)

// mockValidator is a test double for provider.WebhookValidator.
type mockValidator struct {
	event provider.WebhookEvent
	err   error
}

func (m *mockValidator) ValidateWebhook(_ context.Context, _ []byte, _ string) (provider.WebhookEvent, error) {
	return m.event, m.err
}

func post(t *testing.T, h http.Handler, body []byte, signature string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/internal/webhooks/github", bytes.NewReader(body))
	if signature != "" {
		r.Header.Set("X-Hub-Signature-256", signature)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestServeHTTP_missingSignature_returns401(t *testing.T) {
	h := handler.NewWebhookHandler(&mockValidator{})
	w := post(t, h, []byte(`{}`), "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestServeHTTP_invalidSignature_returns401(t *testing.T) {
	h := handler.NewWebhookHandler(&mockValidator{err: provider.ErrInvalidSignature})
	w := post(t, h, []byte(`{}`), "sha256=bad")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestServeHTTP_validSignature_returns200(t *testing.T) {
	h := handler.NewWebhookHandler(&mockValidator{event: provider.WebhookEvent{}})
	w := post(t, h, []byte(`{"action":"push"}`), "sha256=valid")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestServeHTTP_validationError_returns500(t *testing.T) {
	h := handler.NewWebhookHandler(&mockValidator{err: errors.New("unexpected internal error")})
	w := post(t, h, []byte(`{}`), "sha256=anything")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestServeHTTP_oversizedBody_returns413(t *testing.T) {
	h := handler.NewWebhookHandler(&mockValidator{})
	bigBody := bytes.Repeat([]byte("x"), 25*1024*1024+1)
	w := post(t, h, bigBody, "sha256=anything")
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run tests — expect compile failure**

```bash
go test ./internal/handler/...
```

Expected: compile error — `handler.NewWebhookHandler` does not exist yet (old code has the wrong interface).

- [ ] **Step 3: Delete `internal/handler/handler.go`**

```bash
rm internal/handler/handler.go
```

- [ ] **Step 4: Rewrite `internal/handler/webhook.go`**

Replace the entire file:

```go
package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/LegationPro/execos/internal/provider"
)

// maxPayloadBytes is GitHub's documented maximum webhook payload size.
const maxPayloadBytes = 25 * 1024 * 1024 // 25 MiB

// Compile-time guard: WebhookHandler must satisfy http.Handler.
var _ http.Handler = (*WebhookHandler)(nil)

// WebhookHandler handles POST /internal/webhooks/github.
// It validates the HMAC-SHA256 signature before any processing.
type WebhookHandler struct {
	validator provider.WebhookValidator
}

// NewWebhookHandler constructs a WebhookHandler with the given validator.
func NewWebhookHandler(v provider.WebhookValidator) *WebhookHandler {
	return &WebhookHandler{validator: v}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Read one byte over the limit so we can detect oversized payloads.
	// io.LimitReader silently truncates without error, so we must check the length ourselves.
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

	// TODO: pass r.Header.Get("X-GitHub-Event") for event routing once WebhookEvent.EventType is populated
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

- [ ] **Step 5: Run tests — expect pass**

```bash
go test ./internal/handler/... -v
```

Expected: all 5 tests pass.

- [ ] **Step 6: Run all tests**

```bash
go test ./...
```

Expected: all tests pass, no compilation errors.

---

## Task 6: Write server entrypoint

**Files:**
- Create: `cmd/server/main.go`

No unit tests for `main.go`. Verified by compilation and an optional smoke test.

- [ ] **Step 1: Create the server entrypoint**

Create `cmd/server/main.go`:

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

- [ ] **Step 2: Build the server binary**

```bash
go build ./cmd/server/...
```

Expected: exits 0, no output.

- [ ] **Step 3: Run all tests one final time**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 4: Smoke test (optional)**

```bash
GITHUB_APP_ID=1 GITHUB_APP_PRIVATE_KEY=fake GITHUB_APP_WEBHOOK_SECRET=test-secret go run ./cmd/server/main.go &
sleep 1

# Should return 401 (missing signature)
curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/internal/webhooks/github -d '{}'
# Expected output: 401

kill %1
```
