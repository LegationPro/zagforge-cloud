package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWorkerConfig_defaults(t *testing.T) {
	vars := []string{"WORKSPACE_DIR", "ZIGZAG_BIN", "REPORTS_DIR"}
	orig := make(map[string]string)
	for _, k := range vars {
		orig[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range orig {
			os.Setenv(k, v)
		}
	}()

	cfg, err := LoadWorkerConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkspaceDir == "" {
		t.Error("expected non-empty WorkspaceDir default")
	}
	if cfg.ZigzagBin != "zigzag" {
		t.Errorf("expected ZigzagBin %q, got %q", "zigzag", cfg.ZigzagBin)
	}
	if !filepath.IsAbs(cfg.ReportsDir) {
		t.Errorf("expected absolute ReportsDir, got %q", cfg.ReportsDir)
	}
}

func TestLoadWorkerConfig_envOverrides(t *testing.T) {
	os.Setenv("WORKSPACE_DIR", "/tmp/myworkspace")
	os.Setenv("ZIGZAG_BIN", "/usr/local/bin/zigzag")
	os.Setenv("REPORTS_DIR", "/var/reports")
	defer func() {
		os.Unsetenv("WORKSPACE_DIR")
		os.Unsetenv("ZIGZAG_BIN")
		os.Unsetenv("REPORTS_DIR")
	}()

	cfg, err := LoadWorkerConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkspaceDir != "/tmp/myworkspace" {
		t.Errorf("expected WorkspaceDir %q, got %q", "/tmp/myworkspace", cfg.WorkspaceDir)
	}
	if cfg.ZigzagBin != "/usr/local/bin/zigzag" {
		t.Errorf("expected ZigzagBin %q, got %q", "/usr/local/bin/zigzag", cfg.ZigzagBin)
	}
	if cfg.ReportsDir != "/var/reports" {
		t.Errorf("expected ReportsDir %q, got %q", "/var/reports", cfg.ReportsDir)
	}
}

func TestLoadWorkerConfig_relativeReportsDirBecomesAbsolute(t *testing.T) {
	os.Setenv("REPORTS_DIR", "relative/reports")
	defer os.Unsetenv("REPORTS_DIR")

	cfg, err := LoadWorkerConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(cfg.ReportsDir) {
		t.Errorf("expected absolute path, got %q", cfg.ReportsDir)
	}
}
