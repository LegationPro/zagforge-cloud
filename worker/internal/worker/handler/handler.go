package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"github.com/LegationPro/zagforge/shared/go/jobtoken"
	"github.com/LegationPro/zagforge/shared/go/store"
	"github.com/LegationPro/zagforge/worker/internal/worker/executor"
)

// RepoLookup is the subset of store.Queries the handler needs.
type RepoLookup interface {
	GetRepoForJob(ctx context.Context, id pgtype.UUID) (store.GetRepoForJobRow, error)
}

// Handler handles Cloud Tasks HTTP requests to execute jobs.
type Handler struct {
	queries  RepoLookup
	executor *executor.Executor
	signer   *jobtoken.Signer
	log      *zap.Logger
}

// New creates a new Cloud Tasks HTTP handler.
func New(queries RepoLookup, executor *executor.Executor, signer *jobtoken.Signer, log *zap.Logger) *Handler {
	return &Handler{queries: queries, executor: executor, signer: signer, log: log}
}

type runRequest struct {
	JobID    string `json:"job_id"`
	JobToken string `json:"job_token"`
}

// Run handles POST /run from Cloud Tasks.
func (h *Handler) Run(w http.ResponseWriter, r *http.Request) {
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Warn("invalid request body", zap.Error(err))
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.JobID == "" || req.JobToken == "" {
		http.Error(w, "job_id and job_token are required", http.StatusBadRequest)
		return
	}

	// Validate the job token.
	if err := h.signer.Validate(req.JobID, req.JobToken); err != nil {
		h.log.Warn("invalid job token",
			zap.String("job_id", req.JobID),
			zap.Error(err),
		)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse job UUID.
	var jobUUID pgtype.UUID
	if err := jobUUID.Scan(req.JobID); err != nil {
		h.log.Warn("invalid job_id format", zap.String("job_id", req.JobID), zap.Error(err))
		http.Error(w, "invalid job_id", http.StatusBadRequest)
		return
	}

	// Look up repo for this job.
	repo, err := h.queries.GetRepoForJob(r.Context(), jobUUID)
	if err != nil {
		h.log.Error("failed to get repo for job", zap.String("job_id", req.JobID), zap.Error(err))
		http.Error(w, "job lookup failed", http.StatusInternalServerError)
		return
	}

	h.log.Info("executing job from cloud task",
		zap.String("job_id", req.JobID),
		zap.String("repo", repo.FullName),
	)

	// Execute synchronously — Cloud Tasks expects a response.
	// 200 = done (success or handled failure), 500 = retry.
	// Match poller convention: orgID=repo.ID, repoID=repo.ID (same value).
	repoID := repo.ID.String()
	h.executor.Execute(r.Context(), req.JobID, repoID, repoID)

	w.WriteHeader(http.StatusOK)
}
