package config

import (
	"errors"
	"os"
)

// LoadJWT is required by the API, not by the independent migration command.
func LoadJWT() (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		return "", errors.New("JWT_SECRET must contain at least 32 bytes")
	}
	return secret, nil
}
