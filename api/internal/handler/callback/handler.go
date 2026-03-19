package callback

import (
	"context"

	"go.uber.org/zap"

	dbpkg "github.com/LegationPro/zagforge-mvp-impl/api/internal/db"
)

// CloneTokenGenerator creates short-lived GitHub Installation Access Tokens.
type CloneTokenGenerator interface {
	GenerateCloneToken(ctx context.Context, installationID int64) (string, error)
}

// Handler handles worker start/complete callbacks.
type Handler struct {
	db     *dbpkg.DB
	cloner CloneTokenGenerator
	log    *zap.Logger
}

func NewHandler(db *dbpkg.DB, cloner CloneTokenGenerator, log *zap.Logger) *Handler {
	return &Handler{db: db, cloner: cloner, log: log}
}
