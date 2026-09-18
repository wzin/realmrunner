package config

import (
	"os"
	"testing"
)

// clearEnv removes every RealmRunner variable so each case starts from scratch.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"REALMRUNNER_PASSWORD_HASH",
		"REALMRUNNER_MAX_RUNNING",
		"REALMRUNNER_PORT_RANGE",
		"REALMRUNNER_MEMORY_MB",
		"REALMRUNNER_DATA_DIR",
		"REALMRUNNER_JWT_SECRET",
		"REALMRUNNER_BASE_URL",
	} {
		// t.Setenv registers the restore, then unset so defaults apply.
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadRequiresPasswordHash(t *testing.T) {
	clearEnv(t)

	if _, err := Load(); err == nil {
		t.Error("expected an error when REALMRUNNER_PASSWORD_HASH is unset")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("REALMRUNNER_PASSWORD_HASH", "hash")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.MaxRunning != 3 {
		t.Errorf("MaxRunning = %d, want 3", cfg.MaxRunning)
	}
	if cfg.PortRange.Min != 25565 || cfg.PortRange.Max != 25600 {
		t.Errorf("PortRange = %+v, want 25565-25600", cfg.PortRange)
	}
	if cfg.MemoryMB != 2048 {
		t.Errorf("MemoryMB = %d, want 2048", cfg.MemoryMB)
	}
	if cfg.DataDir != "/data" {
		t.Errorf("DataDir = %q, want /data", cfg.DataDir)
	}
}

func TestLoadFromEnvironment(t *testing.T) {
	clearEnv(t)
	t.Setenv("REALMRUNNER_PASSWORD_HASH", "hash")
	t.Setenv("REALMRUNNER_MAX_RUNNING", "7")
	t.Setenv("REALMRUNNER_PORT_RANGE", "26000-26010")
	t.Setenv("REALMRUNNER_MEMORY_MB", "4096")
	t.Setenv("REALMRUNNER_DATA_DIR", "/srv/data")
	t.Setenv("REALMRUNNER_JWT_SECRET", "secret")
	t.Setenv("REALMRUNNER_BASE_URL", "mc.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.MaxRunning != 7 || cfg.MemoryMB != 4096 {
		t.Errorf("unexpected limits: %+v", cfg)
	}
	if cfg.PortRange.Min != 26000 || cfg.PortRange.Max != 26010 {
		t.Errorf("PortRange = %+v, want 26000-26010", cfg.PortRange)
	}
	if cfg.DataDir != "/srv/data" || cfg.JWTSecret != "secret" || cfg.BaseURL != "mc.example.com" {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"max running":      {"REALMRUNNER_MAX_RUNNING": "many"},
		"port range shape": {"REALMRUNNER_PORT_RANGE": "25565"},
		"port range min":   {"REALMRUNNER_PORT_RANGE": "low-25600"},
		"port range max":   {"REALMRUNNER_PORT_RANGE": "25565-high"},
		"memory":           {"REALMRUNNER_MEMORY_MB": "lots"},
	}

	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("REALMRUNNER_PASSWORD_HASH", "hash")
			for k, v := range env {
				t.Setenv(k, v)
			}

			if _, err := Load(); err == nil {
				t.Errorf("expected an error for %s", name)
			}
		})
	}
}

func TestPortRangeContains(t *testing.T) {
	pr := PortRange{Min: 25565, Max: 25600}

	tests := []struct {
		port int
		want bool
	}{
		{25565, true},
		{25600, true},
		{25580, true},
		{25564, false},
		{25601, false},
		{0, false},
	}

	for _, tt := range tests {
		if got := pr.Contains(tt.port); got != tt.want {
			t.Errorf("Contains(%d) = %v, want %v", tt.port, got, tt.want)
		}
	}
}
