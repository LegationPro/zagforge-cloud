package cors

import (
	"net/http"

	"github.com/go-chi/cors"
)

func Cors(allowedOrigins []string) func(http.Handler) http.Handler {
	// If there is no allowedOrigions, simply fallback to zagforge.com
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"https://zagforge.com"}
	}

	return cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	})
}
