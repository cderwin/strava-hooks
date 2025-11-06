package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"

	"github.com/labstack/echo/v4"
)

type TokenResponse struct {
	TokenType    string `json:"token_type"`
	ExpiresAt    int64  `json:"expires_at"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	Athlete      struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
	} `json:"athlete"`
}

func (s *ServerState) handleConnect(c echo.Context) error {
	redirectUrl, err := url.JoinPath(s.config.BaseUrl, "oauth2/callback")
	if err != nil {
		return err
	}

	authorizationUrl, err := url.Parse(authUrl)
	params := authorizationUrl.Query()
	params.Add("client_id", s.config.StravaClientId)
	params.Add("redirect_uri", redirectUrl)
	params.Add("response_type", "code")
	params.Add("scope", "read,activity:read_all")
	authorizationUrl.RawQuery = params.Encode()

	c.Redirect(http.StatusFound, authorizationUrl.String())
	return nil
}

func (s *ServerState) handleCallback(c echo.Context) error {
	// Get the authorization code from query params
	code := c.QueryParam("code")
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "No code in callback")
	}

	// Exchange code for access token
	token, err := exchangeCode(code, &s.config)
	if err != nil {
		slog.Error("failed to exchange code with strava", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to exchange temporary code with strava")
	}

	slog.Info("Token exchange completed for oauth2 callback", "athlete_id", token.Athlete.ID, "athlete_username", token.Athlete.Username, "access_token", token.AccessToken)
	err = s.tokenStore.SaveToken(token.Athlete.ID, TokenInfo{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, ExpiresAt: int(token.ExpiresAt)})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save token to redis")
	}

	// Display success page with token info
	html, err := os.ReadFile("/usr/src/static/confirmation.html")
	if err != nil {
		slog.Error("cannot load confirmation template", "err", err)
		return err
	}

	c.HTMLBlob(http.StatusOK, html)
	return nil
}

func exchangeCode(code string, config *Config) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", config.StravaClientId)
	data.Set("client_secret", config.StravaClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")

	resp, err := http.PostForm(tokenUrl, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed: %s", body)
	}

	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, err
	}

	return &token, nil
}
