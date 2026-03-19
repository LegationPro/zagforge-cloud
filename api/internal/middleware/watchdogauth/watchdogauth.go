package watchdogauth

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/LegationPro/zagforge-mvp-impl/shared/go/httputil"
)

var (
	ErrMissingToken = errors.New("missing authorization token")
	ErrInvalidToken = errors.New("invalid watchdog token")
)

// SharedSecret returns middleware that validates requests using a shared secret
// in the Authorization: Bearer <secret> header.
// This is a simple auth mechanism for dev/staging. In production, replace with
// GCP OIDC token validation for Cloud Scheduler.
func SharedSecret(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !found || token == "" {
				httputil.ErrResponse(w, http.StatusUnauthorized, ErrMissingToken)
				return
			}

			if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
				httputil.ErrResponse(w, http.StatusUnauthorized, ErrInvalidToken)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
