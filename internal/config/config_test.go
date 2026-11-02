package config

import (
	"os"
	"testing"
)

func TestLoadDefaultPort(t *testing.T) {
	// Setenv registers cleanup so even an originally unset variable is restored.
	t.Setenv("PORT", "")
	if err := os.Unsetenv("PORT"); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("Port = %d, want 8080", cfg.Port)
	}
}

func TestLoadPort(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    int
		wantErr bool
	}{
		{name: "custom", value: "9090", want: 9090},
		{name: "minimum", value: "1", want: 1},
		{name: "maximum", value: "65535", want: 65535},
		{name: "empty", value: "", wantErr: true},
		{name: "text", value: "abc", wantErr: true},
		{name: "zero", value: "0", wantErr: true},
		{name: "negative", value: "-1", wantErr: true},
		{name: "out of range", value: "65536", wantErr: true},
		{name: "overflow", value: "999999999999999999999999", wantErr: true},
		{name: "address", value: ":8080", wantErr: true},
		{name: "whitespace", value: " 8080 ", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PORT", tt.value)
			cfg, err := Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && cfg.Port != tt.want {
				t.Fatalf("Port = %d, want %d", cfg.Port, tt.want)
			}
		})
	}
}
