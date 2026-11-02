package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Database struct {
	Host            string
	Port            int
	User            string
	Password        string
	Name            string
	Timeout         time.Duration
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// LoadDatabase is separate so tools need not validate unrelated HTTP settings.
func LoadDatabase() (Database, error) {
	cfg := Database{
		Host:     envDefault("DB_HOST", "127.0.0.1"),
		User:     envDefault("DB_USER", "supportflow"),
		Password: os.Getenv("DB_PASSWORD"),
		Name:     envDefault("DB_NAME", "supportflow"),
	}
	for _, field := range []struct{ name, value string }{
		{"DB_HOST", cfg.Host}, {"DB_USER", cfg.User}, {"DB_PASSWORD", cfg.Password}, {"DB_NAME", cfg.Name},
	} {
		if strings.TrimSpace(field.value) == "" {
			return Database{}, fmt.Errorf("%s must not be empty", field.name)
		}
	}
	var err error
	cfg.Port, err = envInt("DB_PORT", "3306", 1, 65535)
	if err != nil {
		return Database{}, err
	}
	cfg.MaxOpenConns, err = envInt("DB_MAX_OPEN_CONNS", "10", 1, 1000)
	if err != nil {
		return Database{}, err
	}
	cfg.MaxIdleConns, err = envInt("DB_MAX_IDLE_CONNS", "5", 0, cfg.MaxOpenConns)
	if err != nil {
		return Database{}, err
	}
	cfg.Timeout, err = envDuration("DB_TIMEOUT", "3s")
	if err != nil {
		return Database{}, err
	}
	cfg.ConnMaxLifetime, err = envDuration("DB_CONN_MAX_LIFETIME", "30m")
	if err != nil {
		return Database{}, err
	}
	return cfg, nil
}

func envDefault(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func envInt(key, fallback string, min, max int) (int, error) {
	value, err := strconv.Atoi(envDefault(key, fallback))
	if err != nil || value < min || value > max {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, min, max)
	}
	return value, nil
}

func envDuration(key, fallback string) (time.Duration, error) {
	value, err := time.ParseDuration(envDefault(key, fallback))
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return value, nil
}
