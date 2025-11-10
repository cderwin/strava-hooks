package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/browser"
	"github.com/urfave/cli/v3"
)

const serverURL = "https://skintrackr.fly.dev"

func loginCommand() *cli.Command {
	return &cli.Command{
		Name:  "login",
		Usage: "Authenticate with Skintrackr server",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runLogin()
		},
	}
}

func runLogin() error {
	// Generate unique session ID
	sessionID := uuid.New().String()

	// Build OAuth start URL with session_id
	authURL := fmt.Sprintf("%s/token/new?session_id=%s", serverURL, sessionID)

	fmt.Println("Opening browser for authentication...")
	fmt.Printf("If the browser doesn't open, visit: %s\n\n", authURL)

	// Open browser
	if err := browser.OpenURL(authURL); err != nil {
		fmt.Printf("Warning: failed to open browser: %v\n", err)
		fmt.Printf("Please manually open: %s\n\n", authURL)
	}

	// Poll for token
	fmt.Println("Waiting for authentication to complete...")
	token, expiresAt, err := pollForToken(sessionID)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Save to config
	config := &Config{
		Auth: AuthConfig{
			Token:     token,
			ExpiresAt: expiresAt,
		},
	}

	if err := saveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	configPath, _ := getConfigPath()
	fmt.Println("\n✓ Authentication successful!")
	fmt.Printf("Token saved to: %s\n", configPath)
	fmt.Printf("Token expires: %s\n", expiresAt.Format(time.RFC1123))

	return nil
}

type pollResponse struct {
	Status    string `json:"status"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

func pollForToken(sessionID string) (string, time.Time, error) {
	pollURL := fmt.Sprintf("%s/token/poll?session_id=%s", serverURL, sessionID)
	client := &http.Client{Timeout: 10 * time.Second}

	// Poll every 1.5 seconds for up to 90 seconds
	maxAttempts := 60
	pollInterval := 1500 * time.Millisecond

	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err := client.Get(pollURL)
		if err != nil {
			// Network error - continue polling
			time.Sleep(pollInterval)
			continue
		}

		var pollResp pollResponse
		if err := json.NewDecoder(resp.Body).Decode(&pollResp); err != nil {
			resp.Body.Close()
			return "", time.Time{}, fmt.Errorf("failed to parse response: %w", err)
		}
		resp.Body.Close()

		// Check if token is ready
		if resp.StatusCode == http.StatusOK && pollResp.Token != "" {
			expiresAt, err := time.Parse(time.RFC3339, pollResp.ExpiresAt)
			if err != nil {
				return "", time.Time{}, fmt.Errorf("failed to parse expiration time: %w", err)
			}
			return pollResp.Token, expiresAt, nil
		}

		// Still pending - continue polling
		if resp.StatusCode == http.StatusAccepted {
			// Show progress indicator
			dots := attempt % 4
			fmt.Printf("\rWaiting%s", string([]byte{'.', '.', '.'}[:dots+1])+"   ")
			time.Sleep(pollInterval)
			continue
		}

		// Unexpected status
		return "", time.Time{}, fmt.Errorf("unexpected response status: %d", resp.StatusCode)
	}

	return "", time.Time{}, fmt.Errorf("authentication timeout after 90 seconds")
}
