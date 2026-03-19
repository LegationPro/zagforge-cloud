package httputil_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LegationPro/zagforge-mvp-impl/shared/go/httputil"
)

type testData struct {
	Key string `json:"key"`
}

func TestErrResponse_writesJSON(t *testing.T) {
	w := httptest.NewRecorder()
	httputil.ErrResponse(w, http.StatusBadRequest, errors.New("bad input"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}

	var resp httputil.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil || *resp.Error != "bad input" {
		t.Errorf("expected error %q, got %v", "bad input", resp.Error)
	}
}

func TestErrResponse_noNextCursor(t *testing.T) {
	w := httptest.NewRecorder()
	httputil.ErrResponse(w, http.StatusNotFound, errors.New("not found"))

	// Ensure the error envelope has no extra fields leaking.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := raw["next_cursor"]; ok {
		t.Error("error response should not contain next_cursor")
	}
	if _, ok := raw["data"]; ok {
		t.Error("error response should not contain data")
	}
}

func TestOkResponse_writesJSON(t *testing.T) {
	w := httptest.NewRecorder()
	httputil.OkResponse(w, testData{Key: "val"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp httputil.Response[testData]
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("expected nil error, got %v", resp.Error)
	}
	if resp.Data.Key != "val" {
		t.Errorf("expected data key %q, got %q", "val", resp.Data.Key)
	}
}

func TestOkResponse_contentType(t *testing.T) {
	w := httptest.NewRecorder()
	httputil.OkResponse(w, testData{Key: "test"})

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}
