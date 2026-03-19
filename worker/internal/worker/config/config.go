package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultJobTimeout = 5 * time.Minute

type Config struct {
	DatabaseURL  string
	AppEnv       string
	WorkspaceDir string
	ZigzagBin    string
	ReportsDir   string
	JobTimeout   time.Duration
	GitHub       GitHubConfig
}

type GitHubConfig struct {
	AppID         int64
	PrivateKey    []byte
	WebhookSecret string
}

func LoadConfig() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL not set")
	}

	ghAppID := os.Getenv("GITHUB_APP_ID")
	var appID int64
	if _, err := fmt.Sscanf(ghAppID, "%d", &appID); err != nil {
		return nil, fmt.Errorf("invalid GITHUB_APP_ID: %w", err)
	}

	ghKey := os.Getenv("GITHUB_APP_PRIVATE_KEY")
	if ghKey == "" {
		return nil, fmt.Errorf("GITHUB_APP_PRIVATE_KEY not set")
	}
	ghKey = strings.ReplaceAll(ghKey, `\n`, "\n")

	ghSecret := os.Getenv("GITHUB_APP_WEBHOOK_SECRET")
	if ghSecret == "" {
		return nil, fmt.Errorf("GITHUB_APP_WEBHOOK_SECRET not set")
	}

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
		reportsDir = "/data/reports"
	}

	jobTimeout := defaultJobTimeout
	if raw := os.Getenv("JOB_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid JOB_TIMEOUT: %w", err)
		}
		jobTimeout = d
	}

	return &Config{
		DatabaseURL:  dbURL,
		AppEnv:       os.Getenv("APP_ENV"),
		WorkspaceDir: workspaceDir,
		ZigzagBin:    zigzagBin,
		ReportsDir:   reportsDir,
		JobTimeout:   jobTimeout,
		GitHub: GitHubConfig{
			AppID:         appID,
			PrivateKey:    []byte(ghKey),
			WebhookSecret: ghSecret,
		},
	}, nil
}
