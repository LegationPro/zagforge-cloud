package api

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/LegationPro/zagforge-mvp-impl/shared/go/httputil"
)

func (h *Handler) GetRepo(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseUUID(r, "repoID")
	if err != nil {
		httputil.ErrResponse(w, http.StatusBadRequest, ErrInvalidRepoID)
		return
	}

	repo, err := h.db.Queries.GetRepoByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		httputil.ErrResponse(w, http.StatusNotFound, ErrRepoNotFound)
		return
	}
	if err != nil {
		h.log.Error("get repo", zap.Error(err))
		httputil.ErrResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	httputil.OkResponse(w, repo)
}
