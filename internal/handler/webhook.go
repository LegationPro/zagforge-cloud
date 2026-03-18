package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/LegationPro/zagforge-mvp-impl/internal/provider"
)

// maxPayloadBytes is GitHub's documented maximum webhook payload size.
const maxPayloadBytes = 25 * 1024 * 1024 // 25 MiB

// Compile-time guard: WebhookHandler must satisfy http.Handler.
var _ http.Handler = (*WebhookHandler)(nil)

// supportedEvents is the set of GitHub event types this handler acts on.
// All other events are acknowledged with 200 OK but not processed.
var supportedEvents = map[string]bool{
	"push": true,
}

// Dispatcher receives a validated push event and runs the job asynchronously.
type Dispatcher interface {
	Dispatch(ctx context.Context, event provider.WebhookEvent)
}

// WebhookHandler handles POST /internal/webhooks/github.
// It validates the HMAC-SHA256 signature before any processing.
type WebhookHandler struct {
	validator  provider.WebhookValidator
	dispatcher Dispatcher
}

// NewWebhookHandler constructs a WebhookHandler with the given validator and dispatcher.
func NewWebhookHandler(v provider.WebhookValidator, d Dispatcher) *WebhookHandler {
	return &WebhookHandler{validator: v, dispatcher: d}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Fast path: reject missing signature before buffering the body.
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		http.Error(w, "missing signature", http.StatusUnauthorized)
		return
	}

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

	event, err := h.validator.ValidateWebhook(r.Context(), body, signature, r.Header.Get("X-GitHub-Event"))
	if errors.Is(err, provider.ErrInvalidSignature) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "validation error", http.StatusInternalServerError)
		return
	}

	if !supportedEvents[event.EventType] {
		w.WriteHeader(http.StatusOK)
		return
	}

	h.dispatcher.Dispatch(context.Background(), event)
	w.WriteHeader(http.StatusOK)
}
