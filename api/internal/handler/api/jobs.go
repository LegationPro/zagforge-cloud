package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/LegationPro/zagforge-mvp-impl/shared/go/httputil"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/store"
)

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseUUID(r, "jobID")
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrInvalidJobID)
		return
	}

	job, err := h.db.Queries.GetJobByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		httputil.ErrResponse(w, http.StatusNotFound, ErrJobNotFound)
		return
	}
	if err != nil {
		h.log.Error("get job", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	httputil.OkResponse(w, job)
}

func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	repoID, err := httputil.ParseUUID(r, "repoID")
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrInvalidRepoID)
		return
	}

	limit := httputil.ParseLimit(r)

	cursor, err := httputil.ParseCursor(r)
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, err)
		return
	}

	jobs, err := h.db.Queries.ListJobsByRepo(r.Context(), store.ListJobsByRepoParams{
		RepoID:    repoID,
		CreatedAt: cursor,
		Limit:     limit,
	})
	if err != nil {
		h.log.Error("list jobs", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	resp := httputil.Response[[]store.Job]{Data: jobs}
	if len(jobs) == int(limit) {
		last := jobs[len(jobs)-1].CreatedAt.Time.Format(time.RFC3339Nano)
		resp.NextCursor = &last
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}
