package config

import (
	"os"
	"testing"
)

// allEnvVars lists every env var used by the API config.
var allEnvVars = []string{
	"APP_ENV", "ENV_FILE",
	"GITHUB_APP_ID", "GITHUB_APP_SLUG", "GITHUB_APP_WEBHOOK_SECRET", "GITHUB_APP_PRIVATE_KEY",
	"CLERK_SECRET_KEY", "HMAC_SIGNING_KEY", "WATCHDOG_SECRET",
	"PORT",
	"DATABASE_URL", "REDIS_URL",
	"GCS_BUCKET", "GCS_ENDPOINT",
	"CLOUD_TASKS_PROJECT", "CLOUD_TASKS_LOCATION", "CLOUD_TASKS_QUEUE", "CLOUD_TASKS_WORKER_URL", "CLOUD_TASKS_SERVICE_ACCOUNT",
	"HMAC_SIGNING_KEY_PREV",
}

// setEnv saves originals, sets the provided map, and unsets everything else in allEnvVars.
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
		"GITHUB_APP_ID":             "2895893256896859",
		"GITHUB_APP_SLUG":           "test-app",
		"GITHUB_APP_WEBHOOK_SECRET": "secret",
		"GITHUB_APP_PRIVATE_KEY":    "test-private-key",
		"CLERK_SECRET_KEY":          "sk_test_xxx",
		"HMAC_SIGNING_KEY":          "test-hmac-key",
		"WATCHDOG_SECRET":           "test-watchdog-secret",
		"PORT":                      "8080",
		"DATABASE_URL":              "postgres://localhost/test",
		"REDIS_URL":                 "redis://localhost:6379",
		"GCS_BUCKET":                "test-bucket",
	}
}

func TestLoad_success(t *testing.T) {
	setEnv(t, validEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.App.GithubAppID != 2895893256896859 {
		t.Errorf("expected AppID 2895893256896859, got %d", cfg.App.GithubAppID)
	}
	if cfg.App.GithubAppWebhookSecret != "secret" {
		t.Errorf("expected secret %q, got %q", "secret", cfg.App.GithubAppWebhookSecret)
	}
	if cfg.App.GithubAppPrivateKey != "test-private-key" {
		t.Errorf("expected private key %q, got %q", "test-private-key", cfg.App.GithubAppPrivateKey)
	}
	if cfg.App.ClerkSecretKey != "sk_test_xxx" {
		t.Errorf("expected ClerkSecretKey %q, got %q", "sk_test_xxx", cfg.App.ClerkSecretKey)
	}
	if cfg.App.HMACSigningKey != "test-hmac-key" {
		t.Errorf("expected HMACSigningKey %q, got %q", "test-hmac-key", cfg.App.HMACSigningKey)
	}
	if cfg.App.WatchdogSecret != "test-watchdog-secret" {
		t.Errorf("expected WatchdogSecret %q, got %q", "test-watchdog-secret", cfg.App.WatchdogSecret)
	}
	if cfg.Server.Port != "8080" {
		t.Errorf("expected Port %q, got %q", "8080", cfg.Server.Port)
	}
	if cfg.DB.URL != "postgres://localhost/test" {
		t.Errorf("expected DB URL %q, got %q", "postgres://localhost/test", cfg.DB.URL)
	}
	if cfg.Redis.URL != "redis://localhost:6379" {
		t.Errorf("expected Redis URL %q, got %q", "redis://localhost:6379", cfg.Redis.URL)
	}
	if cfg.GCS.Bucket != "test-bucket" {
		t.Errorf("expected GCS Bucket %q, got %q", "test-bucket", cfg.GCS.Bucket)
	}
}

func TestLoad_privateKeyNewlineConversion(t *testing.T) {
	env := validEnv()
	env["GITHUB_APP_PRIVATE_KEY"] = `line1\nline2\nline3`
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "line1\nline2\nline3"
	if cfg.App.GithubAppPrivateKey != expected {
		t.Errorf("expected newlines converted, got %q", cfg.App.GithubAppPrivateKey)
	}
}

func TestLoad_missingRequired(t *testing.T) {
	requiredVars := []string{
		"GITHUB_APP_ID",
		"GITHUB_APP_SLUG",
		"GITHUB_APP_WEBHOOK_SECRET",
		"GITHUB_APP_PRIVATE_KEY",
		"CLERK_SECRET_KEY",
		"HMAC_SIGNING_KEY",
		"WATCHDOG_SECRET",
		"PORT",
		"DATABASE_URL",
		"REDIS_URL",
		"GCS_BUCKET",
	}

	for _, missing := range requiredVars {
		t.Run(missing, func(t *testing.T) {
			env := validEnv()
			delete(env, missing)
			setEnv(t, env)

			_, err := Load()
			if err == nil {
				t.Fatalf("expected error for missing %s", missing)
			}
		})
	}
}

func TestLoad_invalidAppID(t *testing.T) {
	env := validEnv()
	env["GITHUB_APP_ID"] = "not-a-number"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITHUB_APP_ID")
	}
}

func TestLoad_optionalGCSEndpoint(t *testing.T) {
	env := validEnv()
	env["GCS_ENDPOINT"] = "http://localhost:4443"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GCS.Endpoint != "http://localhost:4443" {
		t.Errorf("expected GCS Endpoint %q, got %q", "http://localhost:4443", cfg.GCS.Endpoint)
	}
}

func TestCloudTasksConfig_Enabled_allSet(t *testing.T) {
	c := CloudTasksConfig{
		Project:   "my-project",
		Location:  "us-central1",
		Queue:     "jobs",
		WorkerURL: "https://worker.example.com",
	}
	if !c.Enabled() {
		t.Fatal("expected Enabled() = true when all fields set")
	}
}

func TestCloudTasksConfig_Enabled_partiallySet(t *testing.T) {
	tests := []struct {
		name string
		cfg  CloudTasksConfig
	}{
		{"missing project", CloudTasksConfig{Location: "us", Queue: "q", WorkerURL: "url"}},
		{"missing location", CloudTasksConfig{Project: "p", Queue: "q", WorkerURL: "url"}},
		{"missing queue", CloudTasksConfig{Project: "p", Location: "us", WorkerURL: "url"}},
		{"missing worker_url", CloudTasksConfig{Project: "p", Location: "us", Queue: "q"}},
		{"all empty", CloudTasksConfig{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cfg.Enabled() {
				t.Fatal("expected Enabled() = false when fields missing")
			}
		})
	}
}

func TestLoad_cloudTasksConfig(t *testing.T) {
	env := validEnv()
	env["CLOUD_TASKS_PROJECT"] = "my-project"
	env["CLOUD_TASKS_LOCATION"] = "us-central1"
	env["CLOUD_TASKS_QUEUE"] = "jobs-queue"
	env["CLOUD_TASKS_WORKER_URL"] = "https://worker.run.app"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.CloudTasks.Enabled() {
		t.Fatal("expected CloudTasks to be enabled")
	}
	if cfg.CloudTasks.Project != "my-project" {
		t.Errorf("expected Project %q, got %q", "my-project", cfg.CloudTasks.Project)
	}
	if cfg.CloudTasks.WorkerURL != "https://worker.run.app" {
		t.Errorf("expected WorkerURL %q, got %q", "https://worker.run.app", cfg.CloudTasks.WorkerURL)
	}
}

func TestLoad_cloudTasksNotConfigured(t *testing.T) {
	setEnv(t, validEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CloudTasks.Enabled() {
		t.Fatal("expected CloudTasks to be disabled when env vars not set")
	}
}
