package main

import "github.com/cderwin/strava-hooks/internal/app"

func main() {
	server := app.NewServer()
	server.RunForever()
}
