package config

import (
	"os"
	"path/filepath"
)

type WorkerConfig struct {
	WorkspaceDir string // base dir for temporary clone directories
	ZigzagBin    string // path to the zigzag binary
	ReportsDir   string // absolute path where zigzag writes reports
}

func LoadWorkerConfig() (*WorkerConfig, error) {
	workspaceDir := os.Getenv("WORKSPACE_DIR")
	if workspaceDir == "" {
		workspaceDir = filepath.Join(os.TempDir(), "zagforge-workspace")
	}

	zigzagBin := os.Getenv("ZIGZAG_BIN")
	if zigzagBin == "" {
		zigzagBin = "zigzag"
	}

	reportsDir := os.Getenv("REPORTS_DIR")
	if reportsDir == "" {
		reportsDir = "zigzag-reports"
	}

	// Resolve to absolute so it stays valid when zigzag runs inside a temp clone dir.
	if !filepath.IsAbs(reportsDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		reportsDir = filepath.Join(cwd, reportsDir)
	}

	return &WorkerConfig{
		WorkspaceDir: workspaceDir,
		ZigzagBin:    zigzagBin,
		ReportsDir:   reportsDir,
	}, nil
}
