//go:build integration

package integration_test

import (
	"net/http"
	"testing"
)

func TestCORS_preflightReturnsHeaders(t *testing.T) {
	env := newTestEnv(t)

	req, _ := http.NewRequest(http.MethodOptions, env.server.URL+"/healthz", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS request: %v", err)
	}
	defer resp.Body.Close()

	acao := resp.Header.Get("Access-Control-Allow-Origin")
	if acao != "http://localhost:3000" {
		t.Errorf("expected ACAO http://localhost:3000, got %q", acao)
	}
}

func TestCORS_disallowedOriginGetsNoHeader(t *testing.T) {
	env := newTestEnv(t)

	req, _ := http.NewRequest(http.MethodOptions, env.server.URL+"/healthz", nil)
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS request: %v", err)
	}
	defer resp.Body.Close()

	acao := resp.Header.Get("Access-Control-Allow-Origin")
	if acao != "" {
		t.Errorf("expected no ACAO header for disallowed origin, got %q", acao)
	}
}
