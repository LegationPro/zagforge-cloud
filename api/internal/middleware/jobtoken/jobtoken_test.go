package jobtoken_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	jobtokenmw "github.com/LegationPro/zagforge/api/internal/middleware/jobtoken"
	"github.com/LegationPro/zagforge/shared/go/jobtoken"
)

func signer() *jobtoken.Signer {
	return jobtoken.NewSigner([]byte("test-secret"), 5*time.Minute)
}

func makeRequest(t *testing.T, token, body string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/internal/jobs/start", bytes.NewBufferString(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return httptest.NewRecorder(), r
}

func TestAuth_missingToken_returns401(t *testing.T) {
	mw := jobtokenmw.Auth(signer(), zap.NewNop())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w, r := makeRequest(t, "", `{"job_id":"abc"}`)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_missingJobID_returns400(t *testing.T) {
	s := signer()
	mw := jobtokenmw.Auth(s, zap.NewNop())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token := s.Sign("some-job")
	w, r := makeRequest(t, token, `{"not_job_id":"abc"}`)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAuth_invalidToken_returns401(t *testing.T) {
	mw := jobtokenmw.Auth(signer(), zap.NewNop())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w, r := makeRequest(t, "bad-token:123", `{"job_id":"abc"}`)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_wrongJobID_returns401(t *testing.T) {
	s := signer()
	mw := jobtokenmw.Auth(s, zap.NewNop())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token := s.Sign("job-aaa")
	w, r := makeRequest(t, token, `{"job_id":"job-bbb"}`)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_validToken_passes(t *testing.T) {
	s := signer()
	jobID := "550e8400-e29b-41d4-a716-446655440000"
	mw := jobtokenmw.Auth(s, zap.NewNop())

	var gotJobID string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotJobID = jobtokenmw.JobIDFromContext(r.Context())
		// Verify body is still readable.
		var body struct {
			JobID string `json:"job_id"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.JobID != jobID {
			t.Errorf("body job_id not preserved: got %q", body.JobID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	token := s.Sign(jobID)
	w, r := makeRequest(t, token, `{"job_id":"`+jobID+`"}`)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotJobID != jobID {
		t.Errorf("expected job_id %q in context, got %q", jobID, gotJobID)
	}
}

func TestAuth_expiredToken_returns401(t *testing.T) {
	s := jobtoken.NewSigner([]byte("test-secret"), -1*time.Second)
	mw := jobtokenmw.Auth(s, zap.NewNop())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token := s.Sign("job-123")
	w, r := makeRequest(t, token, `{"job_id":"job-123"}`)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_invalidJSON_returns400(t *testing.T) {
	s := signer()
	mw := jobtokenmw.Auth(s, zap.NewNop())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token := s.Sign("job-123")
	w, r := makeRequest(t, token, `not json`)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
