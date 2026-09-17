package config

import (
	"fmt"
	"strings"
)

type Config struct {
	DiscordToken string
	DatabaseURL  string
	HealthPort   string
	// BotAdminIDs are Discord user IDs treated as the guild owner in every
	// guild the bot is in, regardless of who actually owns that guild.
	BotAdminIDs []string
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DiscordToken: getenv("DISCORD_TOKEN"),
		DatabaseURL:  getenv("DATABASE_URL"),
		HealthPort:   getenv("HEALTH_PORT"),
		BotAdminIDs:  parseIDList(getenv("BOT_ADMIN_IDS")),
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

// parseIDList splits a comma-separated list of IDs, trimming whitespace and
// dropping empty entries.
func parseIDList(raw string) []string {
	var ids []string
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
