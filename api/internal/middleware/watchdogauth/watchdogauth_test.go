package watchdogauth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LegationPro/zagforge-mvp-impl/api/internal/middleware/watchdogauth"
)

const testSecret = "test-watchdog-secret"

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestSharedSecret_validToken(t *testing.T) {
	h := watchdogauth.SharedSecret(testSecret)(okHandler())
	r := httptest.NewRequest(http.MethodPost, "/internal/watchdog/timeout", nil)
	r.Header.Set("Authorization", "Bearer "+testSecret)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSharedSecret_missingToken(t *testing.T) {
	h := watchdogauth.SharedSecret(testSecret)(okHandler())
	r := httptest.NewRequest(http.MethodPost, "/internal/watchdog/timeout", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSharedSecret_wrongToken(t *testing.T) {
	h := watchdogauth.SharedSecret(testSecret)(okHandler())
	r := httptest.NewRequest(http.MethodPost, "/internal/watchdog/timeout", nil)
	r.Header.Set("Authorization", "Bearer wrong-secret")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSharedSecret_noBearerPrefix(t *testing.T) {
	h := watchdogauth.SharedSecret(testSecret)(okHandler())
	r := httptest.NewRequest(http.MethodPost, "/internal/watchdog/timeout", nil)
	r.Header.Set("Authorization", "Token "+testSecret)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
