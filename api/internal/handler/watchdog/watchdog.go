package watchdog

import (
	"context"
	"errors"
	"net/http"

	"go.uber.org/zap"

	dbpkg "github.com/LegationPro/zagforge/api/internal/db"
	"github.com/LegationPro/zagforge/shared/go/httputil"
)

const defaultTimeoutMinutes = 20

var ErrInternal = errors.New("internal error")

// TimeoutResponse is returned after the watchdog runs.
type TimeoutResponse struct {
	TimedOut int64 `json:"timed_out"`
}

// Handler handles the watchdog timeout endpoint.
type Handler struct {
	db             *dbpkg.DB
	timeoutMinutes int32
	log            *zap.Logger
}

func NewHandler(db *dbpkg.DB, log *zap.Logger) *Handler {
	return &Handler{
		db:             db,
		timeoutMinutes: defaultTimeoutMinutes,
		log:            log,
	}
}

// Timeout fails all jobs stuck in "running" beyond the timeout threshold.
// Called by Cloud Scheduler every 5 minutes.
func (h *Handler) Timeout(w http.ResponseWriter, r *http.Request) {
	count, err := h.db.Queries.TimeoutRunningJobs(r.Context(), h.timeoutMinutes)
	if err != nil {
		h.log.Error("watchdog: timeout query failed", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	if count > 0 {
		h.log.Warn("watchdog: timed out stuck jobs",
			zap.Int64("count", count),
			zap.Int32("threshold_minutes", h.timeoutMinutes),
		)
	}

	httputil.WriteJSON(w, http.StatusOK, TimeoutResponse{TimedOut: count})
}

// HealthCheck is a lightweight endpoint for Cloud Scheduler to verify the watchdog is reachable.
func (h *Handler) HealthCheck(w http.ResponseWriter, _ *http.Request) {
	if err := h.db.Pool.Ping(context.Background()); err != nil {
		httputil.ErrResponse(w, http.StatusServiceUnavailable, ErrInternal)
		return
	}
	w.WriteHeader(http.StatusOK)
}
