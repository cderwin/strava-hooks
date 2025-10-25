package main

import "github.com/cderwin/strava-hooks/app"

func main() {
	server := app.NewServer()
	server.RunForever()
}
