package config

type ServerConfig struct {
	Port string `env:"PORT,required"`
}
