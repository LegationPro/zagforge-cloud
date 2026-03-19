package httputil

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	DefaultPageLimit = 50
	MaxPageLimit     = 100
)

var ErrInvalidCursor = errors.New("invalid cursor, expected RFC3339")

// ParseUUID extracts a chi URL param as pgtype.UUID.
func ParseUUID(r *http.Request, param string) (pgtype.UUID, error) {
	raw := chi.URLParam(r, param)
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		return id, err
	}
	return id, nil
}

// ParseLimit reads the "limit" query param, clamped to [1, MaxPageLimit].
func ParseLimit(r *http.Request) int32 {
	s := r.URL.Query().Get("limit")
	if s == "" {
		return DefaultPageLimit
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return DefaultPageLimit
	}
	if n > MaxPageLimit {
		return MaxPageLimit
	}
	return int32(n)
}

// UUIDFromString parses a UUID string into pgtype.UUID.
func UUIDFromString(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(s)
	return id, err
}

// ParseCursor reads the "cursor" query param as an RFC3339 timestamp.
// Returns the current time if no cursor is provided.
func ParseCursor(r *http.Request) (pgtype.Timestamptz, error) {
	cursor := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return cursor, ErrInvalidCursor
		}
		cursor = pgtype.Timestamptz{Time: t, Valid: true}
	}
	return cursor, nil
}
