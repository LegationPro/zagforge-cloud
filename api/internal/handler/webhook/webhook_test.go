package webhook_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LegationPro/zagforge/api/internal/handler/webhook"
	github "github.com/LegationPro/zagforge/shared/go/provider/github"
	"go.uber.org/zap"
)

// mockValidator is a test double for provider.WebhookValidator.
type mockValidator struct {
	event github.WebhookEvent
	err   error
}

func (m *mockValidator) ValidateWebhook(_ context.Context, _ []byte, _ string, _ string) (github.WebhookEvent, error) {
	return m.event, m.err
}

// mockPushHandler replaces mockDispatcher.
type mockPushHandler struct {
	err        error
	called     bool
	deliveryID string
	event      github.WebhookEvent
}

func (m *mockPushHandler) HandlePush(_ context.Context, event github.WebhookEvent, deliveryID string) error {
	m.called = true
	m.event = event
	m.deliveryID = deliveryID
	return m.err
}

func post(t *testing.T, h http.Handler, body []byte, signature, eventType string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/internal/webhooks/github", bytes.NewReader(body))
	if signature != "" {
		r.Header.Set("X-Hub-Signature-256", signature)
	}
	if eventType != "" {
		r.Header.Set("X-GitHub-Event", eventType)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func newHandler(v *mockValidator, svc *mockPushHandler) *webhook.Handler {
	return webhook.NewHandler(v, svc, zap.NewNop())
}

func TestServeHTTP_missingSignature_returns401(t *testing.T) {
	h := newHandler(&mockValidator{}, &mockPushHandler{})
	w := post(t, h, []byte(`{}`), "", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestServeHTTP_invalidSignature_returns401(t *testing.T) {
	h := newHandler(&mockValidator{err: github.ErrInvalidSignature}, &mockPushHandler{})
	w := post(t, h, []byte(`{}`), "sha256=bad", "push")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestServeHTTP_pushEvent_returns202(t *testing.T) {
	svc := &mockPushHandler{}
	h := newHandler(&mockValidator{event: github.WebhookEvent{EventType: "push"}}, svc)
	w := post(t, h, []byte(`{}`), "sha256=valid", "push")
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
}

func TestServeHTTP_pushEvent_callsHandlePush(t *testing.T) {
	event := github.WebhookEvent{EventType: "push", RepoName: "org/repo", Branch: "main"}
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
	h := newHandler(&mockValidator{event: github.WebhookEvent{EventType: "push"}}, svc)
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
	h := newHandler(&mockValidator{event: github.WebhookEvent{EventType: "push"}}, svc)
	w := post(t, h, []byte(`{}`), "sha256=valid", "push")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestServeHTTP_validationError_returns500(t *testing.T) {
	h := newHandler(&mockValidator{err: errors.New("unexpected internal error")}, &mockPushHandler{})
	w := post(t, h, []byte(`{}`), "sha256=anything", "push")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestServeHTTP_unsupportedEvent_returns200(t *testing.T) {
	svc := &mockPushHandler{}
	h := newHandler(&mockValidator{event: github.WebhookEvent{EventType: "ping"}}, svc)
	w := post(t, h, []byte(`{}`), "sha256=valid", "ping")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for unsupported event, got %d", w.Code)
	}
	if svc.called {
		t.Error("expected HandlePush not to be called for unsupported event")
	}
}

func TestServeHTTP_oversizedBody_returns413(t *testing.T) {
	h := newHandler(&mockValidator{}, &mockPushHandler{})
	bigBody := bytes.Repeat([]byte("x"), 25*1024*1024+1)
	w := post(t, h, bigBody, "sha256=anything", "push")
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", w.Code)
	}
}
