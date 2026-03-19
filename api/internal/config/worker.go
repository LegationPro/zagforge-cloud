package config

import (
	"os"
	"path/filepath"
)

type WorkerConfig struct {
	WorkspaceDir string `env:"WORKSPACE_DIR"` // base dir for temporary clone directories
	ZigzagBin    string `env:"ZIGZAG_BIN"    envDefault:"zigzag"`
	ReportsDir   string `env:"REPORTS_DIR"    envDefault:"zigzag-reports"` // absolute path where zigzag writes reports
}

func (w *WorkerConfig) init() error {
	if w.WorkspaceDir == "" {
		w.WorkspaceDir = filepath.Join(os.TempDir(), "zagforge-workspace")
	}

	// Resolve to absolute so it stays valid when zigzag runs inside a temp clone dir.
	if !filepath.IsAbs(w.ReportsDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		w.ReportsDir = filepath.Join(cwd, w.ReportsDir)
	}

	return nil
}
