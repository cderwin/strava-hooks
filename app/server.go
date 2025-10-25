package app

import (
	"context"
	"net/http"
	"log/slog"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const (
	authUrl  = "https://www.strava.com/oauth/authorize"
	tokenUrl = "https://www.strava.com/oauth/token"
)

type ServerState struct {
	config Config
}

func NewServer() ServerState {
	config := LoadConfig()
	return ServerState{config: config}
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
	go EstablishSubscriptions(&s.config)

	slog.Info("starting server", "port", 8080)
	e.Logger.Fatal(e.Start(":8080"))
}

func handleHealthcheck(c echo.Context) error {
	response := struct{
		Ok bool `json:"ok"`
	}{Ok: true}
	c.JSON(http.StatusOK, response)
	return nil
}

