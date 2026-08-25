package config

import "testing"

func validEnv() map[string]string {
	return map[string]string{
		"DISCORD_TOKEN": "token123",
		"DATABASE_URL":  "postgres://localhost/action_items",
	}
}

func loadWithEnv(env map[string]string) (Config, error) {
	return Load(func(key string) string {
		return env[key]
	})
}

func TestLoad_ValidConfig(t *testing.T) {
	cfg, err := loadWithEnv(validEnv())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.DiscordToken != "token123" {
		t.Errorf("DiscordToken = %q, want %q", cfg.DiscordToken, "token123")
	}
	if cfg.DatabaseURL != "postgres://localhost/action_items" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://localhost/action_items")
	}
	if cfg.HealthPort != "8080" {
		t.Errorf("HealthPort = %q, want %q", cfg.HealthPort, "8080")
	}
}

func TestLoad_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		missing string
	}{
		{name: "missing discord token", missing: "DISCORD_TOKEN"},
		{name: "missing database url", missing: "DATABASE_URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv()
			delete(env, tt.missing)

			_, err := loadWithEnv(env)
			if err == nil {
				t.Fatalf("Load() error = nil, want an error about missing %s", tt.missing)
			}
		})
	}
}

func TestLoad_HealthPortDefaultsTo8080(t *testing.T) {
	cfg, err := loadWithEnv(validEnv())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
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
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.HealthPort != "9090" {
		t.Errorf("HealthPort = %q, want %q", cfg.HealthPort, "9090")
	}
}
