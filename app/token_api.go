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
	// Get the authorization code, state, and challenge from query params
	code := c.QueryParam("code")
	state := c.QueryParam("state")
	challengeParam := c.QueryParam("challenge")

	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "No code in callback")
	}
	if state == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "No state in callback")
	}
	if challengeParam == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "No challenge in callback")
	}

	// Retrieve the stored challenge code using the state token
	storedChallenge, err := s.store.GetOAuthState(state)
	if err != nil {
		slog.Error("invalid OAuth state", "err", err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid or expired state token")
	}

	// Verify the challenge matches what was stored
	if challengeParam != storedChallenge {
		slog.Error("challenge mismatch", "provided", challengeParam, "stored", storedChallenge)
		return echo.NewHTTPError(http.StatusForbidden, "Challenge verification failed")
	}

	// Exchange code for access token
	token, err := exchangeCode(code, &s.config, &s.stravaClient)
	if err != nil {
		slog.Error("failed to exchange code with strava", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to exchange temporary code with strava")
	}

	slog.Info("Token exchange completed for token API", "athlete_id", token.Athlete.ID, "athlete_username", token.Athlete.Username, "challenge", storedChallenge)

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
	response := map[string]interface{}{
		"access_token": jwtToken,
		"token_type":   "Bearer",
		"athlete_id":   token.Athlete.ID,
		"challenge":    storedChallenge,
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

	// Check if the token has expired
	if claims.ExpiresAt < time.Now().Unix() {
		slog.Info("token has expired", "expires_at", claims.ExpiresAt, "athlete_id", claims.AthleteID)
		return echo.NewHTTPError(http.StatusUnauthorized, map[string]interface{}{
			"error":      "token_expired",
			"message":    "Token has expired",
			"expires_at": claims.ExpiresAt,
		})
	}

	// Check if the token has been revoked
	revoked, err := s.store.IsJWTRevoked(claims.JTI)
	if err != nil {
		slog.Error("failed to check revocation status", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to verify token status")
	}
	if revoked {
		slog.Info("token has been revoked", "jti", claims.JTI, "athlete_id", claims.AthleteID)
		return echo.NewHTTPError(http.StatusUnauthorized, "Token has been revoked")
	}

	// Return the claims
	response := map[string]interface{}{
		"valid":      true,
		"athlete_id": claims.AthleteID,
		"expires_at": claims.ExpiresAt,
		"issued_at":  claims.IssuedAt.Unix(),
		"jti":        claims.JTI,
	}

	return c.JSON(http.StatusOK, response)
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
	response := map[string]interface{}{
		"revoked":    true,
		"jti":        claims.JTI,
		"athlete_id": claims.AthleteID,
	}

	return c.JSON(http.StatusOK, response)
}
