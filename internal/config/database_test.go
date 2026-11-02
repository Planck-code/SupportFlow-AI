package config

import (
	"testing"
	"time"
)

func setDatabaseEnv(t *testing.T) {
	t.Helper()
	for key, value := range map[string]string{
		"DB_HOST": "127.0.0.1", "DB_PORT": "3306", "DB_USER": "supportflow",
		"DB_PASSWORD": "test-only", "DB_NAME": "supportflow", "DB_TIMEOUT": "3s",
		"DB_MAX_OPEN_CONNS": "10", "DB_MAX_IDLE_CONNS": "5", "DB_CONN_MAX_LIFETIME": "30m",
	} {
		t.Setenv(key, value)
	}
}

func TestLoadDatabase(t *testing.T) {
	setDatabaseEnv(t)
	cfg, err := LoadDatabase()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "127.0.0.1" || cfg.Port != 3306 || cfg.User != "supportflow" || cfg.Name != "supportflow" || cfg.Timeout != 3*time.Second || cfg.MaxOpenConns != 10 || cfg.MaxIdleConns != 5 || cfg.ConnMaxLifetime != 30*time.Minute {
		t.Fatal("unexpected database configuration")
	}
	t.Setenv("DB_PORT", "3307")
	cfg, err = LoadDatabase()
	if err != nil || cfg.Port != 3307 {
		t.Fatalf("custom port: %d, err %v", cfg.Port, err)
	}
}

func TestLoadDatabaseRejectsInvalidConfiguration(t *testing.T) {
	for _, tt := range []struct{ key, value string }{
		{"DB_HOST", ""}, {"DB_USER", ""}, {"DB_PASSWORD", ""}, {"DB_NAME", ""},
		{"DB_PORT", "0"}, {"DB_PORT", "65536"}, {"DB_PORT", "bad"},
		{"DB_MAX_OPEN_CONNS", "0"}, {"DB_MAX_IDLE_CONNS", "-1"}, {"DB_MAX_IDLE_CONNS", "11"},
		{"DB_TIMEOUT", "invalid"}, {"DB_TIMEOUT", "0s"}, {"DB_CONN_MAX_LIFETIME", "-1m"},
	} {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			setDatabaseEnv(t)
			t.Setenv(tt.key, tt.value)
			if _, err := LoadDatabase(); err == nil {
				t.Fatal("invalid database configuration accepted")
			}
		})
	}
}
