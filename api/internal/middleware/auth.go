package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkjwt "github.com/clerk/clerk-sdk-go/v2/jwt"
	"go.uber.org/zap"

	"github.com/LegationPro/zagforge-mvp-impl/shared/go/httputil"
)

type contextKey string

const claimsKey contextKey = "clerk_claims"

var (
	ErrMissingToken   = errors.New("missing authorization token")
	ErrInvalidToken   = errors.New("invalid or expired token")
	ErrClaimsNotFound = errors.New("clerk session claims not found in context")
)

type Response struct {
	Error string `json:"error,omitempty"`
}

// ClaimsFromContext retrieves the Clerk session claims from the request context.
func ClaimsFromContext(ctx context.Context) (*clerk.SessionClaims, error) {
	claims, ok := ctx.Value(claimsKey).(*clerk.SessionClaims)
	if !ok {
		return nil, ErrClaimsNotFound
	}
	return claims, nil
}

// Auth returns middleware that verifies Clerk JWTs on incoming requests.
// Requests without a valid token receive a 401 response.
func Auth(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				httputil.WriteJSON(w, http.StatusUnauthorized, Response{Error: ErrMissingToken.Error()})
				return
			}

			claims, err := clerkjwt.Verify(r.Context(), &clerkjwt.VerifyParams{
				Token: token,
			})
			if err != nil {
				log.Warn("auth: invalid token", zap.Error(err))
				httputil.WriteJSON(w, http.StatusUnauthorized, Response{Error: ErrInvalidToken.Error()})
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractToken(r *http.Request) string {
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found {
		return ""
	}
	return token
}
