package health

import (
	"context"
	"net/http"
	"time"

	"github.com/LegationPro/zagforge-mvp-impl/shared/go/httputil"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool *pgxpool.Pool
}

type Response struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

// Liveness returns 200 if the process is running. No dependency checks.
func (h *Handler) Liveness(w http.ResponseWriter, _ *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, Response{Status: "ok"})
}

// Readiness returns 200 only if the server can serve traffic (DB reachable).
func (h *Handler) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, Response{Status: "unavailable", Reason: "db unreachable"})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, Response{Status: "ready"})
}
