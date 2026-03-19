package api

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/LegationPro/zagforge-mvp-impl/shared/go/httputil"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/store"
)

func (h *Handler) GetSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseUUID(r, "snapshotID")
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrInvalidSnapshotID)
		return
	}

	snap, err := h.db.Queries.GetSnapshotByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		httputil.ErrResponse(w, http.StatusNotFound, ErrSnapshotNotFound)
		return
	}
	if err != nil {
		h.log.Error("get snapshot", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	httputil.OkResponse(w, snap)
}

func (h *Handler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	repoID, err := httputil.ParseUUID(r, "repoID")
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrInvalidRepoID)
		return
	}

	branch := r.URL.Query().Get("branch")
	if branch == "" {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrBranchRequired)
		return
	}

	snaps, err := h.db.Queries.GetSnapshotsByBranch(r.Context(), store.GetSnapshotsByBranchParams{
		RepoID: repoID,
		Branch: branch,
	})
	if err != nil {
		h.log.Error("list snapshots", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	httputil.OkResponse(w, snaps)
}

func (h *Handler) GetLatestSnapshot(w http.ResponseWriter, r *http.Request) {
	repoID, err := httputil.ParseUUID(r, "repoID")
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrInvalidRepoID)
		return
	}

	branch := r.URL.Query().Get("branch")
	if branch == "" {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrBranchRequired)
		return
	}

	snap, err := h.db.Queries.GetLatestSnapshot(r.Context(), store.GetLatestSnapshotParams{
		RepoID: repoID,
		Branch: branch,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		httputil.ErrResponse(w, http.StatusNotFound, ErrSnapshotNotFound)
		return
	}
	if err != nil {
		h.log.Error("get latest snapshot", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	httputil.OkResponse(w, snap)
}
