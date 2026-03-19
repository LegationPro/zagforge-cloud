package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type GCSConfig struct {
	Bucket   string `env:"GCS_BUCKET,required"`
	Endpoint string `env:"GCS_ENDPOINT"`
}

type GitHubConfig struct {
	AppID         int64 `env:"GITHUB_APP_ID,required"`
	PrivateKey    []byte
	PrivateKeyRaw string `env:"GITHUB_APP_PRIVATE_KEY,required"`
	WebhookSecret string `env:"GITHUB_APP_WEBHOOK_SECRET,required"`
}

type Config struct {
	DatabaseURL        string        `env:"DATABASE_URL,required"`
	AppEnv             string        `env:"APP_ENV"`
	WorkspaceDir       string        `env:"WORKSPACE_DIR"`
	ZigzagBin          string        `env:"ZIGZAG_BIN"        envDefault:"zigzag"`
	ReportsDir         string        `env:"REPORTS_DIR"        envDefault:"/data/reports"`
	JobTimeout         time.Duration `env:"JOB_TIMEOUT"        envDefault:"5m"`
	MaxConcurrency     int           `env:"MAX_CONCURRENCY"    envDefault:"5"`
	APIBaseURL         string        `env:"API_BASE_URL,required"`
	HMACSigningKey     string        `env:"HMAC_SIGNING_KEY,required"`
	HMACSigningKeyPrev string        `env:"HMAC_SIGNING_KEY_PREV"`
	WorkerMode         string        `env:"WORKER_MODE"        envDefault:"poll"`
	Port               string        `env:"PORT"               envDefault:"8080"`
	GitHub             GitHubConfig  `envPrefix:""`
	GCS                GCSConfig     `envPrefix:""`
}

func LoadConfig() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.WorkspaceDir == "" {
		cfg.WorkspaceDir = filepath.Join(os.TempDir(), "zagforge-workspace")
	}

	if cfg.MaxConcurrency < 1 {
		return nil, fmt.Errorf("invalid MAX_CONCURRENCY: must be >= 1")
	}

	// Env vars often store PEM keys with literal \n instead of real newlines.
	cfg.GitHub.PrivateKey = []byte(strings.ReplaceAll(cfg.GitHub.PrivateKeyRaw, `\n`, "\n"))

	return &cfg, nil
}
