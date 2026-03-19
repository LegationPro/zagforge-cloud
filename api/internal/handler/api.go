package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	dbpkg "github.com/LegationPro/zagforge-mvp-impl/api/internal/db"
	store "github.com/LegationPro/zagforge-mvp-impl/shared/go/store"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/httputil"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 100
)

var (
	ErrInvalidRepoID     = errors.New("invalid repo id")
	ErrInvalidJobID      = errors.New("invalid job id")
	ErrInvalidSnapshotID = errors.New("invalid snapshot id")
	ErrInvalidCursor     = errors.New("invalid cursor, expected RFC3339")
	ErrRepoNotFound      = errors.New("repo not found")
	ErrJobNotFound       = errors.New("job not found")
	ErrSnapshotNotFound  = errors.New("snapshot not found")
	ErrBranchRequired    = errors.New("branch query param required")
	ErrInternal          = errors.New("internal error")
)

type APIHandler struct {
	db  *dbpkg.DB
	log *zap.Logger
}

type Response struct {
	Data       any     `json:"data,omitempty"`
	Error      *string `json:"error,omitempty"`
	NextCursor *string `json:"next_cursor,omitempty"`
}

func NewAPIHandler(db *dbpkg.DB, log *zap.Logger) *APIHandler {
	return &APIHandler{db: db, log: log}
}

func errResponse(w http.ResponseWriter, status int, err error) {
	httputil.WriteJSON(w, status, Response{Error: new(err.Error())})
}

func okResponse(w http.ResponseWriter, data any) {
	httputil.WriteJSON(w, http.StatusOK, Response{Data: data})
}

// parseUUID extracts a chi URL param as pgtype.UUID.
func parseUUID(r *http.Request, param string) (pgtype.UUID, error) {
	raw := chi.URLParam(r, param)
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		return id, err
	}
	return id, nil
}

// parseLimit reads the "limit" query param, clamped to [1, maxPageLimit].
func parseLimit(r *http.Request) int32 {
	s := r.URL.Query().Get("limit")
	if s == "" {
		return defaultPageLimit
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return defaultPageLimit
	}
	if n > maxPageLimit {
		return maxPageLimit
	}
	return int32(n)
}

// -- Repos --

func (h *APIHandler) GetRepo(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "repoID")
	if err != nil {
		errResponse(w, http.StatusBadRequest, ErrInvalidRepoID)
		return
	}

	repo, err := h.db.Queries.GetRepoByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		errResponse(w, http.StatusNotFound, ErrRepoNotFound)
		return
	}
	if err != nil {
		h.log.Error("get repo", zap.Error(err))
		errResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	okResponse(w, repo)
}

// -- Jobs --

func (h *APIHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "jobID")
	if err != nil {
		errResponse(w, http.StatusBadRequest, ErrInvalidJobID)
		return
	}

	job, err := h.db.Queries.GetJobByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		errResponse(w, http.StatusNotFound, ErrJobNotFound)
		return
	}
	if err != nil {
		h.log.Error("get job", zap.Error(err))
		errResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	okResponse(w, job)
}

func (h *APIHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	repoID, err := parseUUID(r, "repoID")
	if err != nil {
		errResponse(w, http.StatusBadRequest, ErrInvalidRepoID)
		return
	}

	limit := parseLimit(r)

	// Cursor: ISO 8601 timestamp. Default to "now" for first page.
	cursor := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			errResponse(w, http.StatusBadRequest, ErrInvalidCursor)
			return
		}
		cursor = pgtype.Timestamptz{Time: t, Valid: true}
	}

	jobs, err := h.db.Queries.ListJobsByRepo(r.Context(), store.ListJobsByRepoParams{
		RepoID:    repoID,
		CreatedAt: cursor,
		Limit:     limit,
	})
	if err != nil {
		h.log.Error("list jobs", zap.Error(err))
		errResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	resp := Response{Data: jobs}
	if len(jobs) == int(limit) {
		last := jobs[len(jobs)-1].CreatedAt.Time.Format(time.RFC3339Nano)
		resp.NextCursor = &last
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// -- Snapshots --

func (h *APIHandler) GetSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "snapshotID")
	if err != nil {
		errResponse(w, http.StatusBadRequest, ErrInvalidSnapshotID)
		return
	}

	snap, err := h.db.Queries.GetSnapshotByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		errResponse(w, http.StatusNotFound, ErrSnapshotNotFound)
		return
	}
	if err != nil {
		h.log.Error("get snapshot", zap.Error(err))
		errResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	okResponse(w, snap)
}

func (h *APIHandler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	repoID, err := parseUUID(r, "repoID")
	if err != nil {
		errResponse(w, http.StatusBadRequest, ErrInvalidRepoID)
		return
	}

	branch := r.URL.Query().Get("branch")
	if branch == "" {
		errResponse(w, http.StatusBadRequest, ErrBranchRequired)
		return
	}

	snaps, err := h.db.Queries.GetSnapshotsByBranch(r.Context(), store.GetSnapshotsByBranchParams{
		RepoID: repoID,
		Branch: branch,
	})
	if err != nil {
		h.log.Error("list snapshots", zap.Error(err))
		errResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	okResponse(w, snaps)
}

func (h *APIHandler) GetLatestSnapshot(w http.ResponseWriter, r *http.Request) {
	repoID, err := parseUUID(r, "repoID")
	if err != nil {
		errResponse(w, http.StatusBadRequest, ErrInvalidRepoID)
		return
	}

	branch := r.URL.Query().Get("branch")
	if branch == "" {
		errResponse(w, http.StatusBadRequest, ErrBranchRequired)
		return
	}

	snap, err := h.db.Queries.GetLatestSnapshot(r.Context(), store.GetLatestSnapshotParams{
		RepoID: repoID,
		Branch: branch,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		errResponse(w, http.StatusNotFound, ErrSnapshotNotFound)
		return
	}
	if err != nil {
		h.log.Error("get latest snapshot", zap.Error(err))
		errResponse(w, http.StatusInternalServerError, ErrInternal)
		return
	}

	okResponse(w, snap)
}
