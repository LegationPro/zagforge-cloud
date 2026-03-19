package db

import (
	"github.com/jackc/pgx/v5/pgxpool"

	store "github.com/LegationPro/zagforge-mvp-impl/shared/go/store"
)

// DB wraps the connection pool and the sqlc-generated query interface.
type DB struct {
	Pool    *pgxpool.Pool
	Queries *store.Queries
}

// New creates a DB from an existing pool.
func New(pool *pgxpool.Pool) *DB {
	return &DB{Pool: pool, Queries: store.New(pool)}
}
