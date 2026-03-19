package callback

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	jobtokenmw "github.com/LegationPro/zagforge-mvp-impl/api/internal/middleware/jobtoken"
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/validate"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/httputil"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/store"
)

// Start transitions a job from queued → running and returns clone info.
// Idempotent: if already running, returns the same info. If terminal, returns 409.
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	req, err := httputil.DecodeJSON[StartRequest](r.Body)
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrInvalidRequestBody)
		return
	}

	if err := validate.Struct(req); err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, err)
		return
	}

	if req.JobID != jobtokenmw.JobIDFromContext(r.Context()) {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrJobIDMismatch)
		return
	}

	jobUUID, err := httputil.UUIDFromString(req.JobID)
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrInvalidJobID)
		return
	}

	tx, err := h.db.Pool.Begin(r.Context())
	if err != nil {
		h.log.Error("start: begin tx", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck

	qtx := store.New(tx)

	job, err := qtx.GetJobForUpdate(r.Context(), jobUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		httputil.ErrResponse(w, http.StatusNotFound, ErrJobNotFound)
		return
	}
	if err != nil {
		h.log.Error("start: get job", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	// Idempotent: if already running, just return info.
	if job.Status == store.JobStatusRunning {
		if err := tx.Commit(r.Context()); err != nil {
			h.log.Error("start: commit", zap.Error(err))
			httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
			return
		}
		h.respondStart(w, r, job)
		return
	}

	// If terminal, the worker shouldn't be starting it.
	if job.Status.IsTerminal() {
		httputil.ErrResponse(w, http.StatusConflict, fmt.Errorf("%w: %s", ErrJobAlreadyTerminal, job.Status))
		return
	}

	// Transition queued → running.
	if err := qtx.UpdateJobStatus(r.Context(), store.UpdateJobStatusParams{
		ID:     job.ID,
		Status: store.JobStatusRunning,
	}); err != nil {
		h.log.Error("start: update status", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		h.log.Error("start: commit", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	h.log.Info("job started", zap.String("job_id", req.JobID))
	h.respondStart(w, r, job)
}

func (h *Handler) respondStart(w http.ResponseWriter, r *http.Request, job store.Job) {
	repo, err := h.db.Queries.GetRepoForJob(r.Context(), job.ID)
	if err != nil {
		h.log.Error("start: get repo for job", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	cloneToken, err := h.cloner.GenerateCloneToken(r.Context(), repo.InstallationID)
	if err != nil {
		h.log.Error("start: generate clone token", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrFailedToCloneToken)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, StartResponse{
		CommitSHA:      job.CommitSha,
		Branch:         job.Branch,
		RepoFullName:   repo.FullName,
		CloneToken:     cloneToken,
		InstallationID: repo.InstallationID,
	})
}
