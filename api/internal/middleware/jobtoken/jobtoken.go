package jobtoken

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/LegationPro/zagforge-mvp-impl/shared/go/httputil"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/jobtoken"
)

type contextKey string

const jobIDKey contextKey = "job_id"

const maxBodyBytes = 1 * 1024 * 1024 // 1 MB

var (
	ErrMissingToken = errors.New("missing authorization token")
	ErrInvalidToken = errors.New("invalid or expired token")
	ErrInvalidBody  = errors.New("failed to read body")
	ErrMissingJobID = errors.New("missing or invalid job_id")
)

// JobIDFromContext retrieves the validated job ID from the request context.
func JobIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(jobIDKey).(string)
	return v
}

// jobIDPayload is the minimal body structure needed for token validation.
type jobIDPayload struct {
	JobID string `json:"job_id"`
}

// Auth returns middleware that validates signed job tokens on internal callback endpoints.
// It reads the request body to extract job_id, validates the Bearer token against it,
// then resets the body so downstream handlers can read it again.
func Auth(signer *jobtoken.Signer, log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearer(r)
			if token == "" {
				httputil.ErrResponse(w, http.StatusUnauthorized, ErrMissingToken)
				return
			}

			// Read body to extract job_id for token validation.
			body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
			if err != nil {
				httputil.ErrResponse(w, http.StatusBadRequest, ErrInvalidBody)
				return
			}

			payload, err := httputil.DecodeJSON[jobIDPayload](strings.NewReader(string(body)))
			if err != nil || payload.JobID == "" {
				httputil.ErrResponse(w, http.StatusBadRequest, ErrMissingJobID)
				return
			}

			if err := signer.Validate(payload.JobID, token); err != nil {
				log.Warn("jobtoken: invalid token",
					zap.String("job_id", payload.JobID),
					zap.Error(err),
				)
				httputil.ErrResponse(w, http.StatusUnauthorized, ErrInvalidToken)
				return
			}

			// Reset body for downstream handlers and inject job_id into context.
			r.Body = io.NopCloser(strings.NewReader(string(body)))
			ctx := context.WithValue(r.Context(), jobIDKey, payload.JobID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearer(r *http.Request) string {
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found {
		return ""
	}
	return token
}
