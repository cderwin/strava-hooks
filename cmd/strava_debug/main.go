package main

import (
	"context"
	"log"
	"os"

	"github.com/cderwin/strava-hooks/app"
	"github.com/tkrajina/gpxgo/gpx"
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
				Name: "output",
				Aliases: []string{"o"},
				Required: true,
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

func DownloadActivityGpx(ActivityId string, Token string, Path string) error {
	streamPoints, err := app.GetActivityStream(ActivityId, Token)
	if err != nil {
		return err
	}

	gpxConfig := app.GpxConfig{}
	GPX, err := app.BuildGpx(streamPoints, gpxConfig)
	if err != nil {
		return err
	}

	xmlBytes, err := GPX.ToXml(gpx.ToXmlParams{})
	if err != nil {
		return err
	}

	err = os.WriteFile(Path, xmlBytes, 0644)
	return err
}
