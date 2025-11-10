package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

func TestGetConfigPath(t *testing.T) {
	// Test with XDG_CONFIG_HOME set
	t.Run("with XDG_CONFIG_HOME", func(t *testing.T) {
		originalXDG := os.Getenv("XDG_CONFIG_HOME")
		defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

		testDir := "/tmp/test-xdg"
		os.Setenv("XDG_CONFIG_HOME", testDir)

		path, err := getConfigPath()
		if err != nil {
			t.Fatalf("getConfigPath() error = %v", err)
		}

		expected := filepath.Join(testDir, "sktk", "config.toml")
		if path != expected {
			t.Errorf("getConfigPath() = %v, want %v", path, expected)
		}
	})

	// Test without XDG_CONFIG_HOME (fallback to ~/.config)
	t.Run("without XDG_CONFIG_HOME", func(t *testing.T) {
		originalXDG := os.Getenv("XDG_CONFIG_HOME")
		defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

		os.Unsetenv("XDG_CONFIG_HOME")

		path, err := getConfigPath()
		if err != nil {
			t.Fatalf("getConfigPath() error = %v", err)
		}

		homeDir, _ := os.UserHomeDir()
		expected := filepath.Join(homeDir, ".config", "sktk", "config.toml")
		if path != expected {
			t.Errorf("getConfigPath() = %v, want %v", path, expected)
		}
	})
}

func TestConfigIsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "expired token",
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      true,
		},
		{
			name:      "valid token",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      false,
		},
		{
			name:      "token expires in 1 second",
			expiresAt: time.Now().Add(1 * time.Second),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Auth: AuthConfig{
					Token:     "test-token",
					ExpiresAt: tt.expiresAt,
				},
			}

			if got := config.IsExpired(); got != tt.want {
				t.Errorf("Config.IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

	os.Setenv("XDG_CONFIG_HOME", tempDir)

	// Create a test config
	expiresAt := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	config := &Config{
		Auth: AuthConfig{
			Token:     "test-jwt-token",
			ExpiresAt: expiresAt,
		},
	}

	// Save config
	if err := saveConfig(config); err != nil {
		t.Fatalf("saveConfig() error = %v", err)
	}

	// Verify file exists
	configPath, _ := getConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("config file was not created at %s", configPath)
	}

	// Load config
	loadedConfig, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	// Verify loaded config matches saved config
	if loadedConfig.Auth.Token != config.Auth.Token {
		t.Errorf("Token mismatch: got %v, want %v", loadedConfig.Auth.Token, config.Auth.Token)
	}

	// Compare times (truncate to seconds since TOML doesn't preserve nanoseconds)
	if !loadedConfig.Auth.ExpiresAt.Truncate(time.Second).Equal(config.Auth.ExpiresAt.Truncate(time.Second)) {
		t.Errorf("ExpiresAt mismatch: got %v, want %v", loadedConfig.Auth.ExpiresAt, config.Auth.ExpiresAt)
	}
}

func TestLoadConfig_NotFound(t *testing.T) {
	// Create a temporary directory with no config
	tempDir := t.TempDir()
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

	os.Setenv("XDG_CONFIG_HOME", tempDir)

	// Try to load non-existent config
	_, err := loadConfig()
	if err == nil {
		t.Error("loadConfig() expected error for non-existent file, got nil")
	}
}

func TestConfigTOMLFormat(t *testing.T) {
	// Verify TOML marshaling produces expected format
	config := &Config{
		Auth: AuthConfig{
			Token:     "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test",
			ExpiresAt: time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
		},
	}

	data, err := toml.Marshal(config)
	if err != nil {
		t.Fatalf("toml.Marshal() error = %v", err)
	}

	tomlStr := string(data)

	// Verify it contains expected structure
	if !contains(tomlStr, "[auth]") {
		t.Error("TOML should contain [auth] section")
	}
	if !contains(tomlStr, "token =") {
		t.Error("TOML should contain token field")
	}
	if !contains(tomlStr, "expires_at =") {
		t.Error("TOML should contain expires_at field")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
