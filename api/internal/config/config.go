package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

func notSetErr(envVar string) error {
	return fmt.Errorf("%q environment variable not set", envVar)
}

type DBConfig struct {
	URL string
}

type RedisConfig struct {
	URL string
}

type GCSConfig struct {
	Bucket   string
	Endpoint string // override for fake-gcs-server in dev
}

type Config struct {
	App    *AppConfig
	Server *ServerConfig
	DB     *DBConfig
	Redis  *RedisConfig
	GCS    *GCSConfig
}

func Load() (*Config, error) {
	if os.Getenv("APP_ENV") == "dev" {
		if envFile := os.Getenv("ENV_FILE"); envFile != "" {
			err := godotenv.Load(envFile)
			if err != nil {
				return nil, err
			}
		}
	}

	app, err := LoadAppConfig()
	if err != nil {
		return nil, err
	}
	server, err := LoadServerConfig()
	if err != nil {
		return nil, err
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, notSetErr("DATABASE_URL")
	}
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		return nil, notSetErr("REDIS_URL")
	}

	gcsBucket := os.Getenv("GCS_BUCKET")
	if gcsBucket == "" {
		return nil, notSetErr("GCS_BUCKET")
	}

	return &Config{
		App:    app,
		Server: server,
		DB:     &DBConfig{URL: dbURL},
		Redis:  &RedisConfig{URL: redisURL},
		GCS: &GCSConfig{
			Bucket:   gcsBucket,
			Endpoint: os.Getenv("GCS_ENDPOINT"),
		},
	}, nil
}
