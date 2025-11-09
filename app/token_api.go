package app

import (
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"
)

// handleTokenStart initiates the OAuth flow with a challenge code
func (s *ServerState) handleTokenStart(c echo.Context) error {
	challenge := c.QueryParam("challenge")
	if challenge == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "challenge parameter is required")
	}

	// Save the challenge code and get a state token
	state, err := s.store.SaveOAuthState(challenge)
	if err != nil {
		slog.Error("failed to save OAuth state", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to initiate OAuth flow")
	}

	// Build the redirect URL
	redirectUrl, err := url.JoinPath(s.config.BaseUrl, "api/token/callback")
	if err != nil {
		return err
	}

	// Construct Strava authorization URL with state parameter
	authorizationUrl, err := url.Parse(authUrl)
	if err != nil {
		return err
	}

	params := authorizationUrl.Query()
	params.Add("client_id", s.config.StravaClientId)
	params.Add("redirect_uri", redirectUrl)
	params.Add("response_type", "code")
	params.Add("scope", "read,activity:read_all")
	params.Add("state", state)
	authorizationUrl.RawQuery = params.Encode()

	c.Redirect(http.StatusFound, authorizationUrl.String())
	return nil
}

// handleTokenCallback handles the OAuth callback and generates a JWT
func (s *ServerState) handleTokenCallback(c echo.Context) error {
	// Get the authorization code and state from query params
	code := c.QueryParam("code")
	state := c.QueryParam("state")

	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "No code in callback")
	}
	if state == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "No state in callback")
	}

	// Retrieve and verify the challenge code
	challenge, err := s.store.GetOAuthState(state)
	if err != nil {
		slog.Error("invalid OAuth state", "err", err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid or expired state token")
	}

	// Exchange code for access token
	token, err := exchangeCode(code, &s.config, &s.stravaClient)
	if err != nil {
		slog.Error("failed to exchange code with strava", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to exchange temporary code with strava")
	}

	slog.Info("Token exchange completed for token API", "athlete_id", token.Athlete.ID, "athlete_username", token.Athlete.Username, "challenge", challenge)

	// Save the Strava token
	err = s.store.SaveToken(token.Athlete.ID, TokenInfo{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.ExpiresAt,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save token to redis")
	}

	// Generate JWT with 30-day expiration
	jwt, err := GenerateJWT(token.Athlete.ID, s.config.Secret, 30*24*time.Hour)
	if err != nil {
		slog.Error("failed to generate JWT", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to generate access token")
	}

	// Return the JWT
	response := map[string]interface{}{
		"access_token": jwt,
		"token_type":   "Bearer",
		"athlete_id":   token.Athlete.ID,
		"challenge":    challenge,
	}

	return c.JSON(http.StatusOK, response)
}

// handleTokenVerify verifies a JWT token
func (s *ServerState) handleTokenVerify(c echo.Context) error {
	// Get token from Authorization header
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "Authorization header required")
	}

	// Extract token (format: "Bearer <token>")
	var tokenString string
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		tokenString = authHeader[7:]
	} else {
		return echo.NewHTTPError(http.StatusUnauthorized, "Invalid authorization format")
	}

	// Verify the JWT
	claims, err := VerifyJWT(tokenString, s.config.Secret)
	if err != nil {
		slog.Error("JWT verification failed", "err", err)
		return echo.NewHTTPError(http.StatusUnauthorized, "Invalid or expired token")
	}

	// Return the claims
	response := map[string]interface{}{
		"valid":      true,
		"athlete_id": claims.AthleteID,
		"expires_at": claims.ExpiresAt,
		"issued_at":  claims.IssuedAt.Unix(),
	}

	return c.JSON(http.StatusOK, response)
}
