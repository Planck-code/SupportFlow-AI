package config

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAIEndpoint(t *testing.T) {
	for _, tc := range []struct{ base, want string }{
		{"https://model.example/v1", "https://model.example/v1/chat/completions"},
		{"https://model.example/v1/chat/completions/", "https://model.example/v1/chat/completions"},
		{"http://127.0.0.1:8081/", "http://127.0.0.1:8081/chat/completions"},
		{"http://[::1]:8000/proxy/v1/", "http://[::1]:8000/proxy/v1/chat/completions"},
		{"http://localhost", "http://localhost/chat/completions"},
	} {
		cfg := AI{BaseURL: tc.base, Model: "model", APIKey: "test-credential", Timeout: time.Second}
		if got, err := cfg.Endpoint(); got != tc.want || err != nil {
			t.Errorf("%s: endpoint mismatch: %v", tc.base, err)
		}
	}
}

func TestLoadAI(t *testing.T) {
	t.Setenv("AI_BASE_URL", "https://model.example/v1")
	t.Setenv("AI_API_KEY", "test-credential")
	t.Setenv("AI_MODEL", "model")
	t.Setenv("AI_TIMEOUT", "")
	if err := os.Unsetenv("AI_TIMEOUT"); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadAI()
	if err != nil || cfg.Timeout != 20*time.Second {
		t.Fatal("incorrect default timeout")
	}
	t.Setenv("AI_TIMEOUT", "750ms")
	cfg, err = LoadAI()
	if err != nil || cfg.Timeout != 750*time.Millisecond {
		t.Fatal("incorrect custom timeout")
	}

	for _, tc := range []struct{ key, value string }{
		{"AI_BASE_URL", ""}, {"AI_BASE_URL", "relative/v1"}, {"AI_BASE_URL", "ftp://localhost"},
		{"AI_BASE_URL", "http://example.test"}, {"AI_BASE_URL", "http://localhost.example.test"},
		{"AI_BASE_URL", "https://user:test-credential@example.test/v1"},
		{"AI_BASE_URL", "https://example.test/v1?key=test-credential"}, {"AI_BASE_URL", "https://example.test/#fragment"},
		{"AI_BASE_URL", "https://example.test:65536"}, {"AI_MODEL", " "}, {"AI_MODEL", "x\ny"},
		{"AI_API_KEY", ""}, {"AI_API_KEY", "test-credential\r\nBad"},
		{"AI_TIMEOUT", "test-credential"}, {"AI_TIMEOUT", "0s"}, {"AI_TIMEOUT", "-1s"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			_, err := LoadAI()
			if !errors.Is(err, ErrAIConfig) || !strings.Contains(err.Error(), tc.key) || strings.Contains(err.Error(), "test-credential") {
				t.Fatal("unsafe or imprecise config error")
			}
		})
	}
}
