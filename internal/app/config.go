package app

import (
	"log"
	"os"
)

type Config struct {
	BaseUrl            string
	StravaClientId     string
	StravaClientSecret string
}

func LoadConfig() Config {
	baseUrl := os.Getenv("APP_BASE_URL")
	if baseUrl == "" {
		baseUrl = "http://localhost:8080"
	}

	clientId := os.Getenv("STRAVA_CLIENT_ID")
	clientSecret := os.Getenv("STRAVA_CLIENT_SECRET")
	if clientId == "" || clientSecret == "" {
		log.Fatal("STRAVA_CLIENT_ID and STRAVA_CLIENT_SECRET must be set")
	}
	return Config{BaseUrl: baseUrl, StravaClientId: clientId, StravaClientSecret: clientSecret}
}
