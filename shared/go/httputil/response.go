package httputil

import "net/http"

// Response is the standard JSON envelope for API responses.
type Response[T any] struct {
	Data       T       `json:"data,omitempty"`
	Error      *string `json:"error,omitempty"`
	NextCursor *string `json:"next_cursor,omitempty"`
}

// ErrorResponse is the JSON envelope for error-only responses (no data payload).
type ErrorResponse struct {
	Error *string `json:"error,omitempty"`
}

// ErrResponse writes a JSON error response with the given status code.
func ErrResponse(w http.ResponseWriter, status int, err error) {
	msg := err.Error()
	WriteJSON(w, status, ErrorResponse{Error: &msg})
}

// OkResponse writes a JSON success response with the given data.
func OkResponse[T any](w http.ResponseWriter, data T) {
	WriteJSON(w, http.StatusOK, Response[T]{Data: data})
}
