//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/LegationPro/zagforge/shared/go/httputil"
)

func TestGetRepo_validUUID_returns200(t *testing.T) {
	env := newTestEnv(t)
	_, repoID := env.seed(t)

	resp := env.get(t, "/api/v1/repos/"+repoID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestGetRepo_invalidUUID_returns400(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/v1/repos/not-a-uuid")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetRepo_notFound_returns404(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/v1/repos/00000000-0000-0000-0000-000000000099")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestListJobs_returnsJobs(t *testing.T) {
	env := newTestEnv(t)
	_, repoID := env.seed(t)
	env.createJob(t, repoID, "main", "abc123")
	env.createJob(t, repoID, "main", "def456")

	resp := env.get(t, "/api/v1/repos/"+repoID+"/jobs")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body httputil.Response[[]json.RawMessage]
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) < 2 {
		t.Errorf("expected at least 2 jobs, got %d", len(body.Data))
	}
}

func TestGetJob_returnsJob(t *testing.T) {
	env := newTestEnv(t)
	_, repoID := env.seed(t)
	jobID := env.createJob(t, repoID, "main", "abc123")

	resp := env.get(t, "/api/v1/repos/"+repoID+"/jobs/"+jobID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestGetJob_notFound_returns404(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/v1/repos/00000000-0000-0000-0000-000000000001/jobs/00000000-0000-0000-0000-000000000099")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestListSnapshots_emptyBranch_returns400(t *testing.T) {
	env := newTestEnv(t)
	_, repoID := env.seed(t)

	resp := env.get(t, "/api/v1/repos/"+repoID+"/snapshots")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestListSnapshots_withBranch_returns200(t *testing.T) {
	env := newTestEnv(t)
	_, repoID := env.seed(t)

	resp := env.get(t, "/api/v1/repos/"+repoID+"/snapshots?branch=main")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestGetLatestSnapshot_noSnapshots_returns404(t *testing.T) {
	env := newTestEnv(t)
	_, repoID := env.seed(t)

	resp := env.get(t, "/api/v1/repos/"+repoID+"/snapshots/latest?branch=main")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
