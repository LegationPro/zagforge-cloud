package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

func notSetErr(envVar string) error {
	return fmt.Errorf("%q environment variable not set", envVar)
}

type Config struct {
	App    *AppConfig
	Server *ServerConfig
	Worker *WorkerConfig
}

func Load() (*Config, error) {
	if os.Getenv("APP_ENV") == "dev" {

		envFile := os.Getenv("ENV_FILE")
		if envFile == "" {
			return nil, notSetErr("ENV_FILE")
		}

		err := godotenv.Load(os.Getenv("ENV_FILE"))
		if err != nil {
			return nil, err
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
	worker, err := LoadWorkerConfig()
	if err != nil {
		return nil, err
	}
	return &Config{App: app, Server: server, Worker: worker}, nil
}
