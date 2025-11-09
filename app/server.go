package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"
)

var (
	authUrl  = "https://www.strava.com/oauth/authorize"
	tokenUrl = "https://www.strava.com/oauth/token"
)

type ServerState struct {
	config       Config
	store        Store
	stravaClient StravaClient
}

func NewServer() ServerState {
	config := LoadConfig()
	redisOptions, err := redis.ParseURL(config.UpstashRedisUrl)
	if err != nil {
		slog.Error("Cannot parse redis url", "upstash_redis_url", config.UpstashRedisUrl, "err", err)
		panic(err)
	}
	redisClient := redis.NewClient(redisOptions)
	// Create a StravaClient without a token for OAuth and API requests
	stravaClient := NewStravaClient("")
	return ServerState{
		config: config,
		store: Store{
			client:       redisClient,
			ctx:          context.Background(),
			config:       &config,
			stravaClient: &stravaClient,
		},
		stravaClient: stravaClient,
	}
}

func (s *ServerState) RunForever() {
	e := echo.New()

	// static files
	e.Static("/static", "/usr/src/static")
	e.File("/", "/usr/src/static/index.html")

	// dynamic routes
	e.GET("/healthcheck", handleHealthcheck)
	e.GET("/oauth2/connect", s.handleConnect)
	e.GET("/oauth2/callback", s.handleCallback)
	e.GET("/subscriptions/callback", s.handleSubscriptionCallback)
	e.POST("/subscriptions/callback", handlePushEvent)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		HandleError: true, // forwards error to the global error handler, so it can decide appropriate status code
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			if v.Error == nil {
				logger.LogAttrs(context.Background(), slog.LevelInfo, "REQUEST",
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
				)
			} else {
				logger.LogAttrs(context.Background(), slog.LevelError, "REQUEST_ERROR",
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
					slog.String("err", v.Error.Error()),
				)
			}
			return nil
		},
	}))

	slog.Info("Establishing subscriptions in background")
	go EstablishSubscriptions(&s.config, &s.stravaClient)

	slog.Info("starting server", "port", 8080)
	e.Logger.Fatal(e.Start(":8080"))
}

func handleHealthcheck(c echo.Context) error {
	response := struct {
		Ok bool `json:"ok"`
	}{Ok: true}
	c.JSON(http.StatusOK, response)
	return nil
}
