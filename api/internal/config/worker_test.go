package config

import (
	"path/filepath"
	"testing"
)

func TestWorkerConfig_init_defaults(t *testing.T) {
	cfg := &WorkerConfig{
		ZigzagBin:  "zigzag",
		ReportsDir: "zigzag-reports",
	}

	if err := cfg.init(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkspaceDir == "" {
		t.Error("expected non-empty WorkspaceDir default")
	}
	if !filepath.IsAbs(cfg.ReportsDir) {
		t.Errorf("expected absolute ReportsDir, got %q", cfg.ReportsDir)
	}
}

func TestWorkerConfig_init_absoluteReportsDir(t *testing.T) {
	cfg := &WorkerConfig{
		WorkspaceDir: "/tmp/myworkspace",
		ZigzagBin:    "/usr/local/bin/zigzag",
		ReportsDir:   "/var/reports",
	}

	if err := cfg.init(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ReportsDir != "/var/reports" {
		t.Errorf("expected ReportsDir %q, got %q", "/var/reports", cfg.ReportsDir)
	}
}

func TestWorkerConfig_init_relativeReportsDirBecomesAbsolute(t *testing.T) {
	cfg := &WorkerConfig{
		ReportsDir: "relative/reports",
	}

	if err := cfg.init(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(cfg.ReportsDir) {
		t.Errorf("expected absolute path, got %q", cfg.ReportsDir)
	}
}
