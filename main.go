package main

import "github.com/cderwin/skintrackr/app"

func main() {
	server := app.NewServer()
	server.RunForever()
}
