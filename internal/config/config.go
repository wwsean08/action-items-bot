package config

import (
	"fmt"
	"strings"
)

type Config struct {
	DiscordToken         string
	GuildID              string
	ActionItemsChannelID string
	ApproverUserIDs      []string
	ApproverRoleID       string
	DatabaseURL          string
	HealthPort           string
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DiscordToken:         getenv("DISCORD_TOKEN"),
		GuildID:              getenv("GUILD_ID"),
		ActionItemsChannelID: getenv("ACTION_ITEMS_CHANNEL_ID"),
		ApproverRoleID:       getenv("APPROVER_ROLE_ID"),
		DatabaseURL:          getenv("DATABASE_URL"),
		HealthPort:           getenv("HEALTH_PORT"),
	}
	if cfg.HealthPort == "" {
		cfg.HealthPort = "8080"
	}

	if raw := getenv("APPROVER_USER_IDS"); raw != "" {
		for _, id := range strings.Split(raw, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				cfg.ApproverUserIDs = append(cfg.ApproverUserIDs, id)
			}
		}
	}

	if cfg.DiscordToken == "" {
		return Config{}, fmt.Errorf("DISCORD_TOKEN is required")
	}
	if cfg.GuildID == "" {
		return Config{}, fmt.Errorf("GUILD_ID is required")
	}
	if cfg.ActionItemsChannelID == "" {
		return Config{}, fmt.Errorf("ACTION_ITEMS_CHANNEL_ID is required")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.ApproverUserIDs) == 0 && cfg.ApproverRoleID == "" {
		return Config{}, fmt.Errorf("at least one of APPROVER_USER_IDS or APPROVER_ROLE_ID is required")
	}

	return cfg, nil
}
