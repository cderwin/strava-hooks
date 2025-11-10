package main

import (
	"context"
	"log"
	"os"

	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "sktk",
		Usage: "Skintrackr CLI - Interact with your Strava data",
		Commands: []*cli.Command{
			loginCommand(),
			exportGpxCommand(),
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
