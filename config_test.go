package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewConfigHTTPWriteBearerToken(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "legacy config without http section",
			yaml: "device-url: rtuovertcp://127.0.0.1:8899\n",
		},
		{
			name: "configured write bearer token",
			yaml: "http:\n  writeBearerToken: test-secret\n",
			want: "test-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			config, err := NewConfig(path)
			if err != nil {
				t.Fatalf("NewConfig returned an error: %v", err)
			}
			if config.HTTP.WriteBearerToken != tt.want {
				t.Fatalf("HTTP.WriteBearerToken = %q, want %q", config.HTTP.WriteBearerToken, tt.want)
			}
		})
	}
}
