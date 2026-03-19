package config_test

import (
	"os"
	"testing"

	"github.com/LegationPro/zagforge-mvp-impl/worker/internal/worker/config"
)

var allEnvVars = []string{
	"DATABASE_URL", "APP_ENV",
	"GITHUB_APP_ID", "GITHUB_APP_PRIVATE_KEY", "GITHUB_APP_WEBHOOK_SECRET",
	"GCS_BUCKET", "GCS_ENDPOINT",
	"API_BASE_URL", "HMAC_SIGNING_KEY",
	"WORKSPACE_DIR", "ZIGZAG_BIN", "REPORTS_DIR",
	"JOB_TIMEOUT", "MAX_CONCURRENCY",
	"WORKER_MODE", "PORT",
}

func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	originals := make(map[string]string, len(allEnvVars))
	for _, k := range allEnvVars {
		originals[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for k, v := range originals {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	})
	for k, v := range vars {
		os.Setenv(k, v)
	}
}

func validEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":              "postgres://localhost/test",
		"GITHUB_APP_ID":             "12345",
		"GITHUB_APP_PRIVATE_KEY":    "test-key",
		"GITHUB_APP_WEBHOOK_SECRET": "test-secret",
		"GCS_BUCKET":                "test-bucket",
		"API_BASE_URL":              "http://localhost:8080",
		"HMAC_SIGNING_KEY":          "test-hmac-key",
	}
}

func TestLoadConfig_success(t *testing.T) {
	setEnv(t, validEnv())

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost/test" {
		t.Errorf("expected DATABASE_URL %q, got %q", "postgres://localhost/test", cfg.DatabaseURL)
	}
	if cfg.GitHub.AppID != 12345 {
		t.Errorf("expected AppID 12345, got %d", cfg.GitHub.AppID)
	}
	if string(cfg.GitHub.PrivateKey) != "test-key" {
		t.Errorf("expected PrivateKey %q, got %q", "test-key", string(cfg.GitHub.PrivateKey))
	}
	if cfg.GitHub.WebhookSecret != "test-secret" {
		t.Errorf("expected WebhookSecret %q, got %q", "test-secret", cfg.GitHub.WebhookSecret)
	}
}

func TestLoadConfig_defaults(t *testing.T) {
	setEnv(t, validEnv())

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ZigzagBin != "zigzag" {
		t.Errorf("expected default ZigzagBin %q, got %q", "zigzag", cfg.ZigzagBin)
	}
	if cfg.ReportsDir != "/data/reports" {
		t.Errorf("expected default ReportsDir %q, got %q", "/data/reports", cfg.ReportsDir)
	}
}

func TestLoadConfig_envOverrides(t *testing.T) {
	env := validEnv()
	env["WORKSPACE_DIR"] = "/custom/workspace"
	env["ZIGZAG_BIN"] = "/usr/bin/zigzag"
	env["REPORTS_DIR"] = "/custom/reports"
	setEnv(t, env)

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkspaceDir != "/custom/workspace" {
		t.Errorf("expected WorkspaceDir %q, got %q", "/custom/workspace", cfg.WorkspaceDir)
	}
	if cfg.ZigzagBin != "/usr/bin/zigzag" {
		t.Errorf("expected ZigzagBin %q, got %q", "/usr/bin/zigzag", cfg.ZigzagBin)
	}
	if cfg.ReportsDir != "/custom/reports" {
		t.Errorf("expected ReportsDir %q, got %q", "/custom/reports", cfg.ReportsDir)
	}
}

func TestLoadConfig_privateKeyNewlineConversion(t *testing.T) {
	env := validEnv()
	env["GITHUB_APP_PRIVATE_KEY"] = `line1\nline2\nline3`
	setEnv(t, env)

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "line1\nline2\nline3"
	if string(cfg.GitHub.PrivateKey) != expected {
		t.Errorf("expected newlines converted, got %q", string(cfg.GitHub.PrivateKey))
	}
}

func TestLoadConfig_missingRequired(t *testing.T) {
	requiredVars := []string{
		"DATABASE_URL",
		"GITHUB_APP_ID",
		"GITHUB_APP_PRIVATE_KEY",
		"GITHUB_APP_WEBHOOK_SECRET",
		"GCS_BUCKET",
		"API_BASE_URL",
		"HMAC_SIGNING_KEY",
	}

	for _, missing := range requiredVars {
		t.Run(missing, func(t *testing.T) {
			env := validEnv()
			delete(env, missing)
			setEnv(t, env)

			_, err := config.LoadConfig()
			if err == nil {
				t.Fatalf("expected error for missing %s", missing)
			}
		})
	}
}

func TestLoadConfig_invalidAppID(t *testing.T) {
	env := validEnv()
	env["GITHUB_APP_ID"] = "not-a-number"
	setEnv(t, env)

	_, err := config.LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid GITHUB_APP_ID")
	}
}

func TestLoadConfig_jobTimeoutOverride(t *testing.T) {
	env := validEnv()
	env["JOB_TIMEOUT"] = "10m"
	setEnv(t, env)

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JobTimeout.String() != "10m0s" {
		t.Errorf("expected JobTimeout 10m0s, got %s", cfg.JobTimeout)
	}
}

func TestLoadConfig_maxConcurrencyOverride(t *testing.T) {
	env := validEnv()
	env["MAX_CONCURRENCY"] = "10"
	setEnv(t, env)

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxConcurrency != 10 {
		t.Errorf("expected MaxConcurrency 10, got %d", cfg.MaxConcurrency)
	}
}

func TestLoadConfig_workerModeDefaults(t *testing.T) {
	setEnv(t, validEnv())

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkerMode != "poll" {
		t.Errorf("expected default WorkerMode %q, got %q", "poll", cfg.WorkerMode)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected default Port %q, got %q", "8080", cfg.Port)
	}
}

func TestLoadConfig_workerModeHTTP(t *testing.T) {
	env := validEnv()
	env["WORKER_MODE"] = "http"
	env["PORT"] = "9090"
	setEnv(t, env)

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkerMode != "http" {
		t.Errorf("expected WorkerMode %q, got %q", "http", cfg.WorkerMode)
	}
	if cfg.Port != "9090" {
		t.Errorf("expected Port %q, got %q", "9090", cfg.Port)
	}
}
