package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/LegationPro/zagforge/api/internal/middleware/auth"
	"github.com/LegationPro/zagforge/shared/go/httputil"
)

func TestAuth_missingToken_returns401(t *testing.T) {
	mw := auth.Auth(zap.NewNop())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	body, err := httputil.DecodeJSON[httputil.ErrorResponse](w.Body)
	if err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body.Error == nil || *body.Error != auth.ErrMissingToken.Error() {
		t.Errorf("expected %q, got %v", auth.ErrMissingToken, body.Error)
	}
}

func TestAuth_invalidToken_returns401(t *testing.T) {
	mw := auth.Auth(zap.NewNop())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	r.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	body, err := httputil.DecodeJSON[httputil.ErrorResponse](w.Body)
	if err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body.Error == nil || *body.Error != auth.ErrInvalidToken.Error() {
		t.Errorf("expected %q, got %v", auth.ErrInvalidToken, body.Error)
	}
}

func TestAuth_missingBearerPrefix_returns401(t *testing.T) {
	mw := auth.Auth(zap.NewNop())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	r.Header.Set("Authorization", "Token some-value")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_responseIsJSON(t *testing.T) {
	mw := auth.Auth(zap.NewNop())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestClaimsFromContext_noClaims_returnsError(t *testing.T) {
	_, err := auth.ClaimsFromContext(context.Background())
	if err == nil {
		t.Fatal("expected error when no claims in context")
	}
	if err != auth.ErrClaimsNotFound {
		t.Errorf("expected %q, got %q", auth.ErrClaimsNotFound, err)
	}
}
