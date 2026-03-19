package ratelimit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/LegationPro/zagforge-mvp-impl/api/internal/middleware/auth"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/httputil"
)

// RateLimitConfig holds the sliding window parameters.
type RateLimitConfig struct {
	MaxRequests int           // max requests per window
	Window      time.Duration // sliding window duration
}

// keyFunc extracts the rate limit key from a request.
// Authenticated requests use the Clerk user ID; unauthenticated use the client IP.
func keyFunc(r *http.Request, prefix string) string {
	if claims, err := auth.ClaimsFromContext(r.Context()); err == nil {
		return fmt.Sprintf("rl:%s:user:%s", prefix, claims.Subject)
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return fmt.Sprintf("rl:%s:ip:%s", prefix, ip)
}

// RateLimit returns middleware that enforces sliding window rate limiting via Redis.
// prefix differentiates route groups (e.g. "api", "webhook").
func RateLimit(rdb *redis.Client, cfg RateLimitConfig, prefix string, log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r, prefix)

			allowed, remaining, err := checkRateLimit(r.Context(), rdb, key, cfg)
			if err != nil {
				// If Redis is down, allow the request but log the error.
				log.Error("rate limit check failed, allowing request", zap.Error(err))
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(cfg.MaxRequests))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			if !allowed {
				log.Warn("rate limit exceeded", zap.String("key", key))
				retryAfter := int(cfg.Window.Seconds())
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				httputil.WriteJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// checkRateLimit uses a Redis sorted set sliding window.
// Returns (allowed, remaining, error).
func checkRateLimit(ctx context.Context, rdb *redis.Client, key string, cfg RateLimitConfig) (bool, int, error) {
	now := time.Now()
	windowStart := now.Add(-cfg.Window).UnixMicro()
	nowMicro := now.UnixMicro()
	member := strconv.FormatInt(nowMicro, 10)

	pipe := rdb.Pipeline()

	// Remove entries outside the window.
	pipe.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(windowStart, 10))

	// Count current entries.
	countCmd := pipe.ZCard(ctx, key)

	// Add the new request.
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(nowMicro), Member: member})

	// Set expiry so keys don't live forever.
	pipe.Expire(ctx, key, cfg.Window+time.Second)

	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, fmt.Errorf("redis pipeline: %w", err)
	}

	count := int(countCmd.Val())
	remaining := max(cfg.MaxRequests-count-1, 0)

	if count >= cfg.MaxRequests {
		return false, 0, nil
	}

	return true, remaining, nil
}
