package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cderwin/skintrackr/app"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

func exportGpxCommand() *cli.Command {
	return &cli.Command{
		Name:      "export-gpx",
		Usage:     "Export Strava activity to GPX file",
		ArgsUsage: "<activity-id>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output file path (default: <activity-id>.gpx)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return fmt.Errorf("requires exactly one argument: <activity-id>")
			}

			activityID := cmd.Args().Get(0)
			output := cmd.String("output")
			if output == "" {
				output = fmt.Sprintf("%s.gpx", activityID)
			}

			return runExport(activityID, output)
		},
	}
}

func runExport(activityID, outputPath string) error {
	// Load config
	config, err := loadConfig()
	if err != nil {
		return err
	}

	// Check if token is expired
	if config.IsExpired() {
		return fmt.Errorf("authentication token has expired. Please run 'sktk login' again")
	}

	// Check if output file exists
	if _, err := os.Stat(outputPath); err == nil {
		// File exists - check if we should overwrite
		if !shouldOverwrite(outputPath) {
			return fmt.Errorf("file already exists: %s", outputPath)
		}
	}

	// Get Strava token from server
	fmt.Println("Fetching Strava access token...")
	stravaToken, err := fetchStravaToken(config.Auth.Token)
	if err != nil {
		return fmt.Errorf("failed to fetch Strava token: %w", err)
	}

	// Download activity
	fmt.Printf("Downloading activity %s...\n", activityID)
	if err := downloadActivityGPX(activityID, stravaToken, outputPath); err != nil {
		return fmt.Errorf("failed to download activity: %w", err)
	}

	fmt.Printf("✓ Activity exported to: %s\n", outputPath)
	return nil
}

type stravaTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

func fetchStravaToken(jwtToken string) (string, error) {
	url := fmt.Sprintf("%s/api/strava-token", serverURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwtToken))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var tokenResp stravaTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return tokenResp.AccessToken, nil
}

func downloadActivityGPX(activityID string, token string, path string) error {
	client := app.NewStravaClient(token)
	activity, err := client.GetActivity(activityID)
	if err != nil {
		return fmt.Errorf("failed to fetch activity: %w", err)
	}

	startTime, err := time.Parse(time.RFC3339, activity.StartDate)
	if err != nil {
		return fmt.Errorf("failed to parse activity start time: %w", err)
	}

	metadata := app.GpxMetadata{
		Name:           activity.Name,
		Type:           activity.Type,
		Time:           startTime,
		UseHeartRate:   true,
		UseTemperature: true,
	}

	err = client.DownloadActivity(activityID, path, metadata)
	if err != nil {
		return fmt.Errorf("failed to download activity gpx: %w", err)
	}
	return nil
}

// shouldOverwrite checks if we should overwrite the file
// If in a TTY, prompt the user. Otherwise, return false (error out).
func shouldOverwrite(path string) bool {
	// Check if stdin is a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// Not a TTY - don't overwrite
		return false
	}

	// Prompt user
	fmt.Printf("File %s already exists. Overwrite? [y/N]: ", path)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}
