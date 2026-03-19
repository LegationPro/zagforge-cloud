package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"github.com/LegationPro/zagforge/shared/go/jobtoken"
	"github.com/LegationPro/zagforge/shared/go/store"
	"github.com/LegationPro/zagforge/worker/internal/worker/executor"
	"github.com/LegationPro/zagforge/worker/internal/worker/handler"
)

var testUUID = pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Valid: true}

type mockRepoLookup struct {
	repo store.GetRepoForJobRow
	err  error
}

func (m *mockRepoLookup) GetRepoForJob(_ context.Context, _ pgtype.UUID) (store.GetRepoForJobRow, error) {
	return m.repo, m.err
}

func testSigner() *jobtoken.Signer {
	return jobtoken.NewSigner([]byte("test-secret"), 30*time.Minute)
}

func newTestHandler(lookup *mockRepoLookup) *handler.Handler {
	signer := testSigner()
	// Executor with nil deps — Execute will return early (api client nil check).
	exec := executor.NewExecutor(nil, nil, nil, zap.NewNop())
	return handler.New(lookup, exec, signer, zap.NewNop())
}

func postRun(t *testing.T, h *handler.Handler, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Run(w, r)
	return w
}

func TestRun_invalidJSON_returns400(t *testing.T) {
	h := newTestHandler(&mockRepoLookup{})
	w := postRun(t, h, []byte(`not json`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRun_missingFields_returns400(t *testing.T) {
	h := newTestHandler(&mockRepoLookup{})

	tests := []struct {
		name string
		body map[string]string
	}{
		{"missing job_id", map[string]string{"job_token": "tok"}},
		{"missing job_token", map[string]string{"job_id": "id"}},
		{"both empty", map[string]string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := json.Marshal(tt.body)
			w := postRun(t, h, b)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestRun_invalidToken_returns401(t *testing.T) {
	h := newTestHandler(&mockRepoLookup{})
	body, _ := json.Marshal(map[string]string{
		"job_id":    "01020304-0506-0708-090a-0b0c0d0e0f10",
		"job_token": "invalid:token",
	})
	w := postRun(t, h, body)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRun_invalidUUID_returns400(t *testing.T) {
	signer := testSigner()
	jobID := "not-a-uuid"
	token := signer.Sign(jobID)

	h := newTestHandler(&mockRepoLookup{})
	body, _ := json.Marshal(map[string]string{
		"job_id":    jobID,
		"job_token": token,
	})
	w := postRun(t, h, body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRun_repoLookupFails_returns500(t *testing.T) {
	signer := testSigner()
	jobID := "01020304-0506-0708-090a-0b0c0d0e0f10"
	token := signer.Sign(jobID)

	lookup := &mockRepoLookup{err: errors.New("db error")}
	h := newTestHandler(lookup)
	body, _ := json.Marshal(map[string]string{
		"job_id":    jobID,
		"job_token": token,
	})
	w := postRun(t, h, body)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestRun_validRequest_returns200(t *testing.T) {
	signer := testSigner()
	jobID := "01020304-0506-0708-090a-0b0c0d0e0f10"
	token := signer.Sign(jobID)

	lookup := &mockRepoLookup{
		repo: store.GetRepoForJobRow{
			ID:             testUUID,
			FullName:       "org/repo",
			InstallationID: 1,
			GithubRepoID:   42,
		},
	}
	h := newTestHandler(lookup)
	body, _ := json.Marshal(map[string]string{
		"job_id":    jobID,
		"job_token": token,
	})
	w := postRun(t, h, body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
