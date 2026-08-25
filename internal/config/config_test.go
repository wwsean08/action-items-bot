package config

import "testing"

func validEnv() map[string]string {
	return map[string]string{
		"DISCORD_TOKEN":           "token123",
		"GUILD_ID":                "guild123",
		"ACTION_ITEMS_CHANNEL_ID": "channel123",
		"APPROVER_USER_IDS":       "user1,user2",
		"DATABASE_URL":            "postgres://localhost/db",
	}
}

func loadWithEnv(env map[string]string) (Config, error) {
	return Load(func(key string) string { return env[key] })
}

func TestLoad_ValidConfig(t *testing.T) {
	cfg, err := loadWithEnv(validEnv())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DiscordToken != "token123" {
		t.Errorf("DiscordToken = %q, want %q", cfg.DiscordToken, "token123")
	}
	if cfg.GuildID != "guild123" {
		t.Errorf("GuildID = %q, want %q", cfg.GuildID, "guild123")
	}
	if cfg.ActionItemsChannelID != "channel123" {
		t.Errorf("ActionItemsChannelID = %q, want %q", cfg.ActionItemsChannelID, "channel123")
	}
	if len(cfg.ApproverUserIDs) != 2 || cfg.ApproverUserIDs[0] != "user1" || cfg.ApproverUserIDs[1] != "user2" {
		t.Errorf("ApproverUserIDs = %v, want [user1 user2]", cfg.ApproverUserIDs)
	}
	if cfg.DatabaseURL != "postgres://localhost/db" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://localhost/db")
	}
}

func TestLoad_MissingRequiredFields(t *testing.T) {
	requiredKeys := []string{
		"DISCORD_TOKEN",
		"GUILD_ID",
		"ACTION_ITEMS_CHANNEL_ID",
		"DATABASE_URL",
	}

	for _, key := range requiredKeys {
		t.Run(key, func(t *testing.T) {
			env := validEnv()
			delete(env, key)

			_, err := loadWithEnv(env)

			if err == nil {
				t.Fatalf("expected error when %s is missing, got nil", key)
			}
		})
	}
}

func TestLoad_NoApproversConfigured(t *testing.T) {
	env := validEnv()
	delete(env, "APPROVER_USER_IDS")

	_, err := loadWithEnv(env)

	if err == nil {
		t.Fatal("expected error when neither APPROVER_USER_IDS nor APPROVER_ROLE_ID is set")
	}
}

func TestLoad_ApproverRoleIDAloneIsSufficient(t *testing.T) {
	env := validEnv()
	delete(env, "APPROVER_USER_IDS")
	env["APPROVER_ROLE_ID"] = "role123"

	cfg, err := loadWithEnv(env)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.ApproverRoleID != "role123" {
		t.Errorf("ApproverRoleID = %q, want %q", cfg.ApproverRoleID, "role123")
	}
	if len(cfg.ApproverUserIDs) != 0 {
		t.Errorf("ApproverUserIDs = %v, want empty", cfg.ApproverUserIDs)
	}
}

func TestLoad_HealthPortDefaultsTo8080(t *testing.T) {
	cfg, err := loadWithEnv(validEnv())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.HealthPort != "8080" {
		t.Errorf("HealthPort = %q, want %q", cfg.HealthPort, "8080")
	}
}

func TestLoad_HealthPortOverride(t *testing.T) {
	env := validEnv()
	env["HEALTH_PORT"] = "9090"

	cfg, err := loadWithEnv(env)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.HealthPort != "9090" {
		t.Errorf("HealthPort = %q, want %q", cfg.HealthPort, "9090")
	}
}
