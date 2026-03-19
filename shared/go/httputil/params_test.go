package httputil_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/LegationPro/zagforge/shared/go/httputil"
)

func TestParseUUID_valid(t *testing.T) {
	mux := chi.NewRouter()
	var gotErr error
	mux.Get("/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, gotErr = httputil.ParseUUID(r, "id")
	})
	r := httptest.NewRequest(http.MethodGet, "/items/550e8400-e29b-41d4-a716-446655440000", nil)
	mux.ServeHTTP(httptest.NewRecorder(), r)

	if gotErr != nil {
		t.Fatalf("expected no error, got %v", gotErr)
	}
}

func TestParseUUID_invalid(t *testing.T) {
	mux := chi.NewRouter()
	var gotErr error
	mux.Get("/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, gotErr = httputil.ParseUUID(r, "id")
	})
	r := httptest.NewRequest(http.MethodGet, "/items/not-a-uuid", nil)
	mux.ServeHTTP(httptest.NewRecorder(), r)

	if gotErr == nil {
		t.Fatal("expected error for invalid UUID")
	}
}

func TestParseLimit_default(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	if got := httputil.ParseLimit(r); got != httputil.DefaultPageLimit {
		t.Errorf("expected %d, got %d", httputil.DefaultPageLimit, got)
	}
}

func TestParseLimit_valid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?limit=25", nil)
	if got := httputil.ParseLimit(r); got != 25 {
		t.Errorf("expected 25, got %d", got)
	}
}

func TestParseLimit_clampsMax(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?limit=999", nil)
	if got := httputil.ParseLimit(r); got != int32(httputil.MaxPageLimit) {
		t.Errorf("expected %d, got %d", httputil.MaxPageLimit, got)
	}
}

func TestParseLimit_invalidFallsToDefault(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?limit=abc", nil)
	if got := httputil.ParseLimit(r); got != httputil.DefaultPageLimit {
		t.Errorf("expected %d, got %d", httputil.DefaultPageLimit, got)
	}
}

func TestParseCursor_empty(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	cursor, err := httputil.ParseCursor(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cursor.Valid {
		t.Error("expected valid cursor")
	}
}

func TestParseCursor_valid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?cursor=2026-01-01T00:00:00Z", nil)
	cursor, err := httputil.ParseCursor(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor.Time.Year() != 2026 {
		t.Errorf("expected year 2026, got %d", cursor.Time.Year())
	}
}

func TestParseCursor_invalid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?cursor=not-a-date", nil)
	_, err := httputil.ParseCursor(r)
	if err != httputil.ErrInvalidCursor {
		t.Fatalf("expected ErrInvalidCursor, got %v", err)
	}
}
