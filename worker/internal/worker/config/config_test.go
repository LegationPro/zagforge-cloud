package config_test

import (
	"os"
	"testing"

	"github.com/LegationPro/zagforge-mvp-impl/worker/internal/worker/config"
)

func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	originals := make(map[string]string, len(vars))
	for k := range vars {
		originals[k] = os.Getenv(k)
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

func TestLoadConfig_missingDatabaseURL(t *testing.T) {
	env := validEnv()
	delete(env, "DATABASE_URL")
	setEnv(t, env)
	os.Unsetenv("DATABASE_URL")

	_, err := config.LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
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

func TestLoadConfig_missingPrivateKey(t *testing.T) {
	env := validEnv()
	delete(env, "GITHUB_APP_PRIVATE_KEY")
	setEnv(t, env)
	os.Unsetenv("GITHUB_APP_PRIVATE_KEY")

	_, err := config.LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing GITHUB_APP_PRIVATE_KEY")
	}
}

func TestLoadConfig_missingWebhookSecret(t *testing.T) {
	env := validEnv()
	delete(env, "GITHUB_APP_WEBHOOK_SECRET")
	setEnv(t, env)
	os.Unsetenv("GITHUB_APP_WEBHOOK_SECRET")

	_, err := config.LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing GITHUB_APP_WEBHOOK_SECRET")
	}
}
