package httputil

import (
	"encoding/json"
	"io"
	"net/http"
)

// WriteJSON serializes v as JSON and writes it to w with the given status code.
func WriteJSON[T any](w http.ResponseWriter, status int, v T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// DecodeJSON reads JSON from r into a value of type T.
func DecodeJSON[T any](r io.Reader) (T, error) {
	var v T
	err := json.NewDecoder(r).Decode(&v)
	return v, err
}
