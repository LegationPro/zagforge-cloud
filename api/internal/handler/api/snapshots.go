package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/LegationPro/zagforge/shared/go/httputil"
	"github.com/LegationPro/zagforge/shared/go/store"
)

// parseBranch extracts and validates the branch query param.
func parseBranch(r *http.Request) (string, error) {
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	if branch == "" {
		return "", ErrBranchRequired
	}
	if len(branch) > maxBranchLength {
		return "", ErrBranchTooLong
	}
	return branch, nil
}

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

	branch, err := parseBranch(r)
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, err)
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

	branch, err := parseBranch(r)
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, err)
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
