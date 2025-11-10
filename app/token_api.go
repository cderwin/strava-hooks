package app

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"
)

type AuthTokenInfo struct {
	token     string
	valid     bool
	athleteId int
	expiresAt time.Time
	issuedAt  time.Time
	jti       string
}

// http request handlers

// handleTokenStart initiates the OAuth flow
func (s *ServerState) handleTokenStart(c echo.Context) error {
	// Generate and save a state token for CSRF protection
	state, err := s.store.SaveOAuthState()
	if err != nil {
		slog.Error("failed to save OAuth state", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to initiate OAuth flow")
	}

	// Build the redirect URL
	redirectUrl, err := url.JoinPath(s.config.BaseUrl, "token/callback")
	if err != nil {
		slog.Error("error building redirect url", "base_url", s.config.BaseUrl, "err", err)
		return fmt.Errorf("error building callback url: %w", err)
	}

	// Construct Strava authorization URL with state parameter
	authorizationUrl, err := url.Parse(authUrl)
	if err != nil {
		slog.Error("failed to parse auth url", "auth_url", authUrl, "err", err)
		return fmt.Errorf("error parsing url: %w", err)
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

	// Verify the state token (CSRF protection)
	err := s.store.GetOAuthState(state)
	if err != nil {
		slog.Error("invalid OAuth state", "err", err)
		return echo.NewHTTPError(http.StatusForbidden, "Invalid or expired state token")
	}

	// Exchange code for access token
	token, err := exchangeCode(code, &s.config, &s.stravaClient)
	if err != nil {
		slog.Error("failed to exchange code with strava", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to exchange temporary code with strava")
	}

	slog.Info("Token exchange completed for token API", "athlete_id", token.Athlete.ID, "athlete_username", token.Athlete.Username)

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
	expirationDuration := 30 * 24 * time.Hour
	jwtToken, jti, err := GenerateJWT(token.Athlete.ID, s.config.Secret, expirationDuration)
	if err != nil {
		slog.Error("failed to generate JWT", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to generate access token")
	}

	// Store JWT metadata in Redis for revocation tracking
	issuedAt := time.Now()
	expiresAt := issuedAt.Add(expirationDuration)
	err = s.store.SaveJWTToken(jti, token.Athlete.ID, issuedAt, expiresAt)
	if err != nil {
		slog.Error("failed to save JWT metadata", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to save token metadata")
	}

	// Return the JWT
	response := map[string]any{
		"access_token": jwtToken,
		"token_type":   "Bearer",
		"athlete_id":   token.Athlete.ID,
	}

	return c.JSON(http.StatusOK, response)
}

// handleTokenVerify verifies a JWT token
func (s *ServerState) handleTokenVerify(c echo.Context) error {
	tokenInfo, err := s.AuthenticateRequest(c.Request())
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err)
	}

	return c.JSON(http.StatusOK, tokenInfo)
}

// handleTokenRevoke revokes a JWT token
func (s *ServerState) handleTokenRevoke(c echo.Context) error {
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

	// Verify the JWT to get the JTI
	claims, err := VerifyJWT(tokenString, s.config.Secret)
	if err != nil {
		slog.Error("JWT verification failed for revocation", "err", err)
		return echo.NewHTTPError(http.StatusUnauthorized, "Invalid or expired token")
	}

	// Check if already revoked
	revoked, err := s.store.IsJWTRevoked(claims.JTI)
	if err != nil {
		slog.Error("failed to check revocation status", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check token status")
	}
	if revoked {
		return echo.NewHTTPError(http.StatusBadRequest, "Token is already revoked")
	}

	// Revoke the token
	err = s.store.RevokeJWTToken(claims.JTI)
	if err != nil {
		slog.Error("failed to revoke token", "err", err, "jti", claims.JTI)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to revoke token")
	}

	// Return success
	response := map[string]any{
		"revoked":    true,
		"jti":        claims.JTI,
		"athlete_id": claims.AthleteID,
	}

	return c.JSON(http.StatusOK, response)
}

func (s *ServerState) handleStravaToken(c echo.Context) error {
	tokenInfo, err := s.AuthenticateRequest(c.Request())
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err)
	}

	stravaToken, err := s.store.fetchTokenInfo(tokenInfo.athleteId)
	if err != nil {
		slog.Error("error fetching strava token", "ethlete_id", tokenInfo.athleteId, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}
	return c.JSON(http.StatusOK, stravaToken)
}

// public functions

func (s *ServerState) AuthenticateRequest(request *http.Request) (AuthTokenInfo, error) {
	// Get token from Authorization header
	authHeader := request.Header.Get("Authorization")
	if authHeader == "" {
		return AuthTokenInfo{}, echo.NewHTTPError(http.StatusUnauthorized, "Authorization header required")
	}

	// Extract token (format: "Bearer <token>")
	var bearerToken string
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		bearerToken = authHeader[7:]
	} else {
		return AuthTokenInfo{}, echo.NewHTTPError(http.StatusUnauthorized, "Invalid authorization format")
	}

	tokenInfo, err := s.AuthenticateToken(bearerToken)
	if err != nil {
		return tokenInfo, echo.NewHTTPError(http.StatusForbidden, err)
	}

	return tokenInfo, nil
}

func (s *ServerState) AuthenticateToken(bearerToken string) (AuthTokenInfo, error) {
	// Verify the JWT
	claims, err := VerifyJWT(bearerToken, s.config.Secret)
	if err != nil {
		return AuthTokenInfo{}, errors.New("Invalid or expired token")
	}

	token := AuthTokenInfo{
		token:     bearerToken,
		valid:     false, // false until proven otherwise
		athleteId: claims.AthleteID,
		expiresAt: time.Unix(claims.ExpiresAt, 0),
		issuedAt:  claims.IssuedAt.Time,
		jti:       claims.JTI,
	}

	// Check if the token has expired
	if token.expiresAt.Unix() < time.Now().Unix() {
		return token, errors.New("token has expired")
	}

	// Check if the token has been revoked
	revoked, err := s.store.IsJWTRevoked(claims.JTI)
	if err != nil {
		slog.Error("error checking revocation status", "err", err)
		return token, errors.New("failed to verify token revocation status")
	}
	if revoked {
		return token, errors.New("token has been revoked")
	}

	token.valid = true
	return token, nil
}

// helpers
