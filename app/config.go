package app

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
)

type Config struct {
	BaseUrl            string
	StravaClientId     string
	StravaClientSecret string
	VerifyToken        string
	UpstashRedisUrl    string
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
		slog.Error("STRAVA_CLIENT_ID and STRAVA_CLIENT_SECRET must be set")
		panic("invalid configuration")
	}

	upstashRedisUrl := os.Getenv("UPSTASH_REDIS_URL")
	if upstashRedisUrl == "" {
		slog.Error("UPSTASH_REDIS_URL environment variable must be set")
		panic("invalid configuration")
	}
	return Config{BaseUrl: baseUrl, StravaClientId: clientId, StravaClientSecret: clientSecret, VerifyToken: randomString(16), UpstashRedisUrl: upstashRedisUrl}
}
