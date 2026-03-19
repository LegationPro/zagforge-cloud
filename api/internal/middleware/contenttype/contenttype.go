package contenttype

import (
	"errors"
	"mime"
	"net/http"

	"github.com/LegationPro/zagforge/shared/go/httputil"
)

var ErrUnsupportedMediaType = errors.New("Content-Type must be application/json")

// RequireJSON returns middleware that rejects POST/PUT/PATCH requests
// without a valid application/json Content-Type header.
// GET, DELETE, OPTIONS, and HEAD requests are passed through.
func RequireJSON() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if requiresBody(r.Method) && !isJSON(r) {
				httputil.ErrResponse(w, http.StatusUnsupportedMediaType, ErrUnsupportedMediaType)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requiresBody(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

func isJSON(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return mediaType == "application/json"
}
