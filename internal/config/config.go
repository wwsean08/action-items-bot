package config

import "fmt"

type Config struct {
	DiscordToken string
	DatabaseURL  string
	HealthPort   string
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DiscordToken: getenv("DISCORD_TOKEN"),
		DatabaseURL:  getenv("DATABASE_URL"),
		HealthPort:   getenv("HEALTH_PORT"),
	}
	if cfg.HealthPort == "" {
		cfg.HealthPort = "8080"
	}

	if cfg.DiscordToken == "" {
		return Config{}, fmt.Errorf("DISCORD_TOKEN is required")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return cfg, nil
}
