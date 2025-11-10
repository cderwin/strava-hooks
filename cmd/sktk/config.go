package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Config represents the CLI configuration stored in TOML format
type Config struct {
	Auth AuthConfig `toml:"auth"`
}

// AuthConfig holds authentication information
type AuthConfig struct {
	Token     string    `toml:"token"`
	ExpiresAt time.Time `toml:"expires_at"`
}

// getConfigPath returns the path to the config file following XDG spec
func getConfigPath() (string, error) {
	// Check XDG_CONFIG_HOME first
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		// Fallback to ~/.config
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		configHome = filepath.Join(homeDir, ".config")
	}

	configDir := filepath.Join(configHome, "sktk")
	configPath := filepath.Join(configDir, "config.toml")

	return configPath, nil
}

// loadConfig reads the config file and returns the Config struct
func loadConfig() (*Config, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found. Please run 'sktk login' first")
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// saveConfig writes the config to the TOML file
func saveConfig(cfg *Config) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to TOML
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// IsExpired checks if the auth token has expired
func (c *Config) IsExpired() bool {
	return time.Now().After(c.Auth.ExpiresAt)
}
