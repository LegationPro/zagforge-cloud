package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	dbpkg "github.com/LegationPro/zagforge-mvp-impl/api/internal/db"
	"github.com/LegationPro/zagforge-mvp-impl/api/internal/handler/api"
	"github.com/LegationPro/zagforge-mvp-impl/shared/go/httputil"
)

// newHandler creates a Handler with a nil DB pool (sufficient for param-validation tests).
func newHandler() *api.Handler {
	return api.NewHandler(&dbpkg.DB{}, zap.NewNop())
}

// chiRequest builds an httptest request routed through a chi mux so URL params resolve.
func chiRequest(t *testing.T, method, pattern, target string, h http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	mux := chi.NewRouter()
	switch method {
	case http.MethodGet:
		mux.Get(pattern, h)
	}
	r := httptest.NewRequest(method, target, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func decodeError(t *testing.T, w *httptest.ResponseRecorder) httputil.ErrorResponse {
	t.Helper()
	resp, err := httputil.DecodeJSON[httputil.ErrorResponse](w.Body)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

// -- Content-Type tests --

func TestHandler_responses_haveJSONContentType(t *testing.T) {
	h := newHandler()
	w := chiRequest(t, http.MethodGet, "/api/v1/repos/{repoID}", "/api/v1/repos/not-a-uuid", h.GetRepo)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

// -- GetRepo tests --

func TestGetRepo_invalidUUID_returns400(t *testing.T) {
	h := newHandler()
	w := chiRequest(t, http.MethodGet, "/api/v1/repos/{repoID}", "/api/v1/repos/not-a-uuid", h.GetRepo)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	resp := decodeError(t, w)
	if resp.Error == nil || *resp.Error != api.ErrInvalidRepoID.Error() {
		t.Errorf("expected error %q, got %v", api.ErrInvalidRepoID, resp.Error)
	}
}

// -- GetJob tests --

func TestGetJob_invalidUUID_returns400(t *testing.T) {
	h := newHandler()
	w := chiRequest(t, http.MethodGet, "/api/v1/repos/{repoID}/jobs/{jobID}", "/api/v1/repos/xxx/jobs/bad-id", h.GetJob)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	resp := decodeError(t, w)
	if resp.Error == nil || *resp.Error != api.ErrInvalidJobID.Error() {
		t.Errorf("expected error %q, got %v", api.ErrInvalidJobID, resp.Error)
	}
}

// -- ListJobs tests --

func TestListJobs_invalidRepoUUID_returns400(t *testing.T) {
	h := newHandler()
	w := chiRequest(t, http.MethodGet, "/api/v1/repos/{repoID}/jobs", "/api/v1/repos/bad/jobs", h.ListJobs)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	resp := decodeError(t, w)
	if resp.Error == nil || *resp.Error != api.ErrInvalidRepoID.Error() {
		t.Errorf("expected error %q, got %v", api.ErrInvalidRepoID, resp.Error)
	}
}

func TestListJobs_invalidCursor_returns400(t *testing.T) {
	h := newHandler()
	w := chiRequest(t, http.MethodGet,
		"/api/v1/repos/{repoID}/jobs",
		"/api/v1/repos/00000000-0000-0000-0000-000000000001/jobs?cursor=not-a-date",
		h.ListJobs,
	)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	resp := decodeError(t, w)
	if resp.Error == nil || *resp.Error != httputil.ErrInvalidCursor.Error() {
		t.Errorf("expected error %q, got %v", httputil.ErrInvalidCursor, resp.Error)
	}
}

// -- GetSnapshot tests --

func TestGetSnapshot_invalidUUID_returns400(t *testing.T) {
	h := newHandler()
	w := chiRequest(t, http.MethodGet, "/api/v1/snapshots/{snapshotID}", "/api/v1/snapshots/nope", h.GetSnapshot)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	resp := decodeError(t, w)
	if resp.Error == nil || *resp.Error != api.ErrInvalidSnapshotID.Error() {
		t.Errorf("expected error %q, got %v", api.ErrInvalidSnapshotID, resp.Error)
	}
}

// -- ListSnapshots tests --

func TestListSnapshots_invalidRepoUUID_returns400(t *testing.T) {
	h := newHandler()
	w := chiRequest(t, http.MethodGet,
		"/api/v1/repos/{repoID}/snapshots",
		"/api/v1/repos/bad/snapshots?branch=main",
		h.ListSnapshots,
	)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListSnapshots_missingBranch_returns400(t *testing.T) {
	h := newHandler()
	w := chiRequest(t, http.MethodGet,
		"/api/v1/repos/{repoID}/snapshots",
		"/api/v1/repos/00000000-0000-0000-0000-000000000001/snapshots",
		h.ListSnapshots,
	)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	resp := decodeError(t, w)
	if resp.Error == nil || *resp.Error != api.ErrBranchRequired.Error() {
		t.Errorf("expected error %q, got %v", api.ErrBranchRequired, resp.Error)
	}
}

// -- GetLatestSnapshot tests --

func TestGetLatestSnapshot_invalidRepoUUID_returns400(t *testing.T) {
	h := newHandler()
	w := chiRequest(t, http.MethodGet,
		"/api/v1/repos/{repoID}/snapshots/latest",
		"/api/v1/repos/bad/snapshots/latest?branch=main",
		h.GetLatestSnapshot,
	)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetLatestSnapshot_missingBranch_returns400(t *testing.T) {
	h := newHandler()
	w := chiRequest(t, http.MethodGet,
		"/api/v1/repos/{repoID}/snapshots/latest",
		"/api/v1/repos/00000000-0000-0000-0000-000000000001/snapshots/latest",
		h.GetLatestSnapshot,
	)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	resp := decodeError(t, w)
	if resp.Error == nil || *resp.Error != api.ErrBranchRequired.Error() {
		t.Errorf("expected error %q, got %v", api.ErrBranchRequired, resp.Error)
	}
}

// -- Response shape tests --

func TestErrResponse_shape(t *testing.T) {
	h := newHandler()
	w := chiRequest(t, http.MethodGet, "/api/v1/repos/{repoID}", "/api/v1/repos/bad", h.GetRepo)

	resp := decodeError(t, w)
	if resp.Error == nil {
		t.Error("expected non-nil error on error response")
	}
}
