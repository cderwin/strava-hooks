package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cderwin/skintrackr/app"
	"github.com/urfave/cli/v3"
)

func main() {
	var token string
	var activityId string
	var outputPath string

	cli := &cli.Command{
		Name:  "strava-debug",
		Usage: "Debug tool for strava-tools application",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "token",
				Aliases:     []string{"t"},
				Required:    true,
				Destination: &token,
				Sources:     cli.EnvVars("STRAVA_TOKEN"),
			},
			&cli.StringFlag{
				Name:        "activity-id",
				Aliases:     []string{"a"},
				Required:    true,
				Destination: &activityId,
			},
			&cli.StringFlag{
				Name:        "output",
				Aliases:     []string{"o"},
				Required:    true,
				Destination: &outputPath,
			},
		},
		Action: func(context.Context, *cli.Command) error {
			err := DownloadActivityGpx(activityId, token, outputPath)
			if err != nil {
				panic(err)
			}
			return nil
		},
	}

	if err := cli.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func DownloadActivityGpx(activityId string, token string, path string) error {
	client := app.NewStravaClient(token)
	activity, err := client.GetActivity(activityId)
	if err != nil {
		panic(fmt.Errorf("failed to fetch activity: %w", err))
	}

	startTime, err := time.Parse(time.RFC3339, activity.StartDate)
	if err != nil {
		panic(fmt.Errorf("failed to parse activity start time: %w", err))
	}

	metadata := app.GpxMetadata{
		Name:           activity.Name,
		Type:           activity.Type,
		Time:           startTime,
		UseHeartRate:   true,
		UseTemperature: true,
	}

	err = client.DownloadActivity(activityId, path, metadata)
	if err != nil {
		panic(fmt.Errorf("failed to download activity gpx: %w", err))
	}
	return nil
}
