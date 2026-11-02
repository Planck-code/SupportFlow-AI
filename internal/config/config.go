package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port int
}

// Load reads process environment variables; it does not load .env files.
func Load() (Config, error) {
	value, exists := os.LookupEnv("PORT")
	if !exists {
		return Config{Port: 8080}, nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("PORT must be an integer between 1 and 65535")
	}
	return Config{Port: port}, nil
}
