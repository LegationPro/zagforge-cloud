package webhook

import (
	"context"
	"errors"
	"io"
	"net/http"

	github "github.com/LegationPro/zagforge-mvp-impl/shared/go/provider/github"
	"go.uber.org/zap"
)

const maxPayloadBytes = 25 * 1024 * 1024

var _ http.Handler = (*Handler)(nil)

var supportedEvents = map[string]bool{
	"push": true,
}

var (
	ErrMissingSignature = errors.New("missing signature")
	ErrInvalidSignature = errors.New("invalid signature")
	ErrFailedToReadBody = errors.New("failed to read body")
	ErrPayloadTooLarge  = errors.New("payload too large")
	ErrValidation       = errors.New("validation error")
	ErrInternal         = errors.New("internal error")
)

// PushHandler receives a validated push event and delivery ID.
// JobService satisfies this interface.
type PushHandler interface {
	HandlePush(ctx context.Context, event github.WebhookEvent, deliveryID string) error
}

type Handler struct {
	validator github.WebhookValidator
	svc       PushHandler
	log       *zap.Logger
}

func NewHandler(v github.WebhookValidator, svc PushHandler, log *zap.Logger) *Handler {
	return &Handler{validator: v, svc: svc, log: log}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		http.Error(w, ErrMissingSignature.Error(), http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxPayloadBytes+1))
	if err != nil {
		http.Error(w, ErrFailedToReadBody.Error(), http.StatusInternalServerError)
		return
	}
	if int64(len(body)) > maxPayloadBytes {
		http.Error(w, ErrPayloadTooLarge.Error(), http.StatusRequestEntityTooLarge)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	event, err := h.validator.ValidateWebhook(r.Context(), body, signature, eventType)
	if errors.Is(err, github.ErrInvalidSignature) {
		h.log.Warn("invalid signature", zap.String("event", eventType))
		http.Error(w, ErrInvalidSignature.Error(), http.StatusUnauthorized)
		return
	}
	if err != nil {
		h.log.Error("validation error", zap.String("event", eventType), zap.Error(err))
		http.Error(w, ErrValidation.Error(), http.StatusInternalServerError)
		return
	}

	if !supportedEvents[event.EventType] {
		h.log.Info("ignoring unsupported event", zap.String("event", event.EventType))
		w.WriteHeader(http.StatusOK)
		return
	}

	deliveryID := r.Header.Get("X-GitHub-Delivery")
	h.log.Info("dispatching webhook",
		zap.String("event", event.EventType),
		zap.String("repo", event.RepoName),
		zap.String("branch", event.Branch),
		zap.String("commit", event.CommitSHA),
	)

	if err := h.svc.HandlePush(r.Context(), event, deliveryID); err != nil {
		h.log.Error("handle push failed", zap.Error(err))
		http.Error(w, ErrInternal.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
