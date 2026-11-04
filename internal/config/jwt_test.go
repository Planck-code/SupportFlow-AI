package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadJWT(t *testing.T) {
	for _, value := range []string{"", "short", strings.Repeat("x", 31), strings.Repeat("x", 32), strings.Repeat("密", 11)} {
		t.Run("bytes", func(t *testing.T) {
			t.Setenv("JWT_SECRET", value)
			secret, err := LoadJWT()
			if (err == nil) != (len(value) >= 32) {
				t.Fatal("incorrect secret length validation")
			}
			if err == nil && secret != value {
				t.Fatal("secret silently modified")
			}
			if err != nil && value != "" && strings.Contains(err.Error(), value) {
				t.Fatal("secret leaked")
			}
		})
	}
	t.Setenv("JWT_SECRET", "temporary")
	os.Unsetenv("JWT_SECRET")
	if _, err := LoadJWT(); err == nil {
		t.Fatal("missing JWT_SECRET accepted")
	}
}
