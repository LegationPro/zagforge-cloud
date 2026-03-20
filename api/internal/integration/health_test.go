//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHealthz_returns200(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/healthz")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %q", body["status"])
	}
}

func TestReadyz_returns200(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/readyz")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ready" {
		t.Errorf("expected status ready, got %q", body["status"])
	}
}
