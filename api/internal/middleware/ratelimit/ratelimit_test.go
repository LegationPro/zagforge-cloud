package ratelimit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/LegationPro/zagforge/api/internal/middleware/ratelimit"
)

func setupRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestRateLimit_allowsRequestsUnderLimit(t *testing.T) {
	rdb := setupRedis(t)
	cfg := ratelimit.RateLimitConfig{MaxRequests: 5, Window: 1 * time.Minute}
	mw := ratelimit.RateLimit(rdb, cfg, "test", zap.NewNop())

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for range 5 {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.RemoteAddr = "1.2.3.4:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	}
}

func TestRateLimit_blocksAfterLimit(t *testing.T) {
	rdb := setupRedis(t)
	cfg := ratelimit.RateLimitConfig{MaxRequests: 3, Window: 1 * time.Minute}
	mw := ratelimit.RateLimit(rdb, cfg, "test", zap.NewNop())

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust the limit.
	for range 3 {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.RemoteAddr = "1.2.3.4:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
	}

	// Next request should be blocked.
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestRateLimit_differentIPsHaveSeparateLimits(t *testing.T) {
	rdb := setupRedis(t)
	cfg := ratelimit.RateLimitConfig{MaxRequests: 2, Window: 1 * time.Minute}
	mw := ratelimit.RateLimit(rdb, cfg, "test", zap.NewNop())

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust limit for IP A.
	for range 2 {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.RemoteAddr = "1.1.1.1:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
	}

	// IP A is blocked.
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.RemoteAddr = "1.1.1.1:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected IP A blocked (429), got %d", w.Code)
	}

	// IP B should still be allowed.
	r = httptest.NewRequest(http.MethodGet, "/test", nil)
	r.RemoteAddr = "2.2.2.2:5678"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected IP B allowed (200), got %d", w.Code)
	}
}

func TestRateLimit_setsRateLimitHeaders(t *testing.T) {
	rdb := setupRedis(t)
	cfg := ratelimit.RateLimitConfig{MaxRequests: 10, Window: 1 * time.Minute}
	mw := ratelimit.RateLimit(rdb, cfg, "test", zap.NewNop())

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Header().Get("X-RateLimit-Limit") != "10" {
		t.Errorf("expected X-RateLimit-Limit 10, got %q", w.Header().Get("X-RateLimit-Limit"))
	}
	if w.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("expected X-RateLimit-Remaining header")
	}
}

func TestRateLimit_responseIsJSON(t *testing.T) {
	rdb := setupRedis(t)
	cfg := ratelimit.RateLimitConfig{MaxRequests: 1, Window: 1 * time.Minute}
	mw := ratelimit.RateLimit(rdb, cfg, "test", zap.NewNop())

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request passes.
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	// Second is blocked.
	r = httptest.NewRequest(http.MethodGet, "/test", nil)
	r.RemoteAddr = "1.2.3.4:1234"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestRateLimit_differentPrefixesAreSeparate(t *testing.T) {
	rdb := setupRedis(t)
	cfg := ratelimit.RateLimitConfig{MaxRequests: 1, Window: 1 * time.Minute}

	webhookMW := ratelimit.RateLimit(rdb, cfg, "webhook", zap.NewNop())
	apiMW := ratelimit.RateLimit(rdb, cfg, "api", zap.NewNop())

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	webhookHandler := webhookMW(ok)
	apiHandler := apiMW(ok)

	// Exhaust webhook limit.
	r := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	r.RemoteAddr = "1.2.3.4:1234"
	w := httptest.NewRecorder()
	webhookHandler.ServeHTTP(w, r)

	// API should still work (different prefix).
	r = httptest.NewRequest(http.MethodGet, "/api", nil)
	r.RemoteAddr = "1.2.3.4:1234"
	w = httptest.NewRecorder()
	apiHandler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected API still allowed (200), got %d", w.Code)
	}
}
