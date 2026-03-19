package callback

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	jobtokenmw "github.com/LegationPro/zagforge-mvp-impl/api/internal/middleware/jobtoken"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/httputil"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/store"
)

// Complete transitions a job to succeeded/failed and optionally inserts a snapshot.
// Idempotent: if already in a terminal state, returns 200 OK as a no-op.
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	req, err := httputil.DecodeJSON[CompleteRequest](io.LimitReader(r.Body, 1*1024*1024))
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrInvalidRequestBody)
		return
	}

	if req.JobID != jobtokenmw.JobIDFromContext(r.Context()) {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrJobIDMismatch)
		return
	}

	if !req.Status.IsValid() {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrInvalidStatus)
		return
	}

	jobUUID, err := httputil.UUIDFromString(req.JobID)
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrInvalidJobID)
		return
	}

	tx, err := h.db.Pool.Begin(r.Context())
	if err != nil {
		h.log.Error("complete: begin tx", zap.Error(err))
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
		h.log.Error("complete: get job", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	// Idempotent: already terminal → no-op.
	if job.Status.IsTerminal() {
		h.log.Info("complete: job already terminal, no-op",
			zap.String("job_id", req.JobID),
			zap.String("status", string(job.Status)),
		)
		tx.Commit(r.Context()) //nolint:errcheck
		httputil.WriteJSON(w, http.StatusOK, StatusResponse{Status: string(job.Status)})
		return
	}

	status := store.JobStatus(req.Status)

	// Update job status.
	if err := qtx.UpdateJobStatus(r.Context(), store.UpdateJobStatusParams{
		ID:           job.ID,
		Status:       status,
		ErrorMessage: pgtype.Text{String: req.ErrorMessage, Valid: req.ErrorMessage != ""},
	}); err != nil {
		h.log.Error("complete: update status", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	// Insert snapshot on success.
	if status == store.JobStatusSucceeded && req.SnapshotPath != "" {
		if _, err := qtx.InsertSnapshot(r.Context(), store.InsertSnapshotParams{
			RepoID:          job.RepoID,
			JobID:           job.ID,
			Branch:          job.Branch,
			CommitSha:       job.CommitSha,
			GcsPath:         req.SnapshotPath,
			SnapshotVersion: 1,
			ZigzagVersion:   req.ZigzagVersion,
			SizeBytes:       req.SizeBytes,
		}); err != nil {
			h.log.Error("complete: insert snapshot", zap.Error(err))
			httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		h.log.Error("complete: commit", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	h.log.Info("job completed",
		zap.String("job_id", req.JobID),
		zap.String("status", req.Status.String()),
	)
	httputil.WriteJSON(w, http.StatusOK, StatusResponse{Status: req.Status.String()})
}
