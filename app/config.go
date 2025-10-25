package app

import (
	"encoding/hex"
	"log"
	"os"
	"crypto/rand"
)

type Config struct {
	BaseUrl            string
	StravaClientId     string
	StravaClientSecret string
	VerifyToken string
}

func randomString(byteLength int) string {
	bytes := make([]byte, byteLength)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
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
	return Config{BaseUrl: baseUrl, StravaClientId: clientId, StravaClientSecret: clientSecret, VerifyToken: randomString(16)}
}
