package api

import (
	"errors"

	"go.uber.org/zap"

	dbpkg "github.com/LegationPro/zagforge-mvp-impl/api/internal/db"
)

var (
	ErrInvalidRepoID     = errors.New("invalid repo id")
	ErrInvalidJobID      = errors.New("invalid job id")
	ErrInvalidSnapshotID = errors.New("invalid snapshot id")
	ErrRepoNotFound      = errors.New("repo not found")
	ErrJobNotFound       = errors.New("job not found")
	ErrSnapshotNotFound  = errors.New("snapshot not found")
	ErrBranchRequired    = errors.New("branch query param required")
	ErrInternal          = errors.New("internal error")
)

type Handler struct {
	db  *dbpkg.DB
	log *zap.Logger
}

func NewHandler(db *dbpkg.DB, log *zap.Logger) *Handler {
	return &Handler{db: db, log: log}
}
