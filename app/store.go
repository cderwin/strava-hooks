package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type TokenInfo struct {
	AccessToken  string `json:"access_token" redis:"access_token"`
	RefreshToken string `json:"refresh_token" redis:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at" redis:"expires_at"`
}

type Store struct {
	client       *redis.Client
	ctx          context.Context
	config       *Config
	stravaClient *StravaClient
}

func (s *Store) FetchToken(AthleteId int) (string, error) {
	tokenInfo, err := s.fetchTokenInfo(AthleteId)
	if err != nil {
		return "", err
	}

	return tokenInfo.AccessToken, nil
}

func (s *Store) SaveToken(athleteId int, token TokenInfo) error {
	authKey := fmt.Sprintf("athlete:%d:strava-token", athleteId)
	expiresAtString := fmt.Sprintf("%d", token.ExpiresAt)

	encryptedAccessToken, err := Encrypt(token.AccessToken, s.config.Secret)
	if err != nil {
		return fmt.Errorf("failed to encrypt access token: %w", err) }

	encryptedRefreshToken, err := Encrypt(token.RefreshToken, s.config.Secret)
	if err != nil {
		return fmt.Errorf("failed to encrypt refresh token: %w", err)
	}

	err = s.client.HSet(s.ctx, authKey, "access_token", encryptedAccessToken, "refresh_token", encryptedRefreshToken, "expires_at", expiresAtString).Err()
	if err != nil {
		slog.Error("error saving token", "err", err)
		return err
	}

	slog.Info("saved new token", "athlete_id", athleteId)
	return nil
}

func (s *Store) fetchTokenInfo(athleteId int) (*TokenInfo, error) {
	authKey := fmt.Sprintf("athlete:%d:strava-token", athleteId)
	var tokenInfo TokenInfo
	err := s.client.HMGet(s.ctx, authKey, "access_token", "refresh_token", "expires_at").Scan(&tokenInfo)
	if err != nil {
		if err == redis.Nil {
			slog.Error("fetch token error: athlete not found", "athlete_id", athleteId)
			return nil, err
		}

		slog.Error("fetch token error: redis request failed", "err", err)
		return nil, err
	}

	tokenInfo.AccessToken, err = Decrypt(tokenInfo.AccessToken, s.config.Secret)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt access token: %w", err)
	}

	tokenInfo.RefreshToken, err = Decrypt(tokenInfo.RefreshToken, s.config.Secret)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt refresh token: %w", err)
	}

	if int64(tokenInfo.ExpiresAt) < time.Now().Unix() {
		slog.Info("token expired, refreshing token", "athlete_id", athleteId)
		newTokenInfo, err := s.refreshToken(athleteId, tokenInfo)
		if err != nil {
			return nil, err
		}
		return newTokenInfo, nil
	}

	return &tokenInfo, nil
}

func (s *Store) refreshToken(AthleteId int, Token TokenInfo) (*TokenInfo, error) {
	formData := map[string]string{
		"client_id":     s.config.StravaClientId,
		"client_secret": s.config.StravaClientSecret,
		"grant_type":    "refresh_token",
		"refresh_token": Token.RefreshToken,
	}

	body, err := s.stravaClient.performRequestForm("POST", tokenUrl, formData)
	if err != nil {
		slog.Error("error refreshing token", "err", err)
		return nil, err
	}

	var newToken TokenInfo
	err = json.NewDecoder(body).Decode(&newToken)
	if err != nil {
		slog.Error("error decoding refresh token response", "err", err)
		return nil, err
	}

	s.SaveToken(AthleteId, newToken)
	return &newToken, nil
}

// generateStateToken creates a random state token
func generateStateToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// SaveOAuthState stores a state token in Redis for CSRF protection
// If sessionID is provided, it encodes it in the state token for CLI polling
// Returns the state token (format: "token" or "token:sessionID")
func (s *Store) SaveOAuthState(sessionID string) (string, error) {
	state := generateStateToken()
	key := fmt.Sprintf("oauth:state:%s", state)

	// Store the session ID if provided, otherwise just store timestamp
	var value string
	if sessionID != "" {
		value = sessionID
	} else {
		value = fmt.Sprintf("%d", time.Now().Unix())
	}

	// Store the state with a 10-minute expiration
	err := s.client.Set(s.ctx, key, value, 10*time.Minute).Err()
	if err != nil {
		return "", fmt.Errorf("failed to save OAuth state: %w", err)
	}

	// If session ID provided, encode it in the returned state
	if sessionID != "" {
		state = fmt.Sprintf("%s:%s", state, sessionID)
	}

	return state, nil
}

// GetOAuthState verifies and deletes a state token
// Returns the session ID if one was encoded in the state
func (s *Store) GetOAuthState(state string) (string, error) {
	// Extract session ID from state if present (format: "token:sessionID")
	var stateToken, sessionID string
	parts := []byte(state)
	colonIndex := -1
	for i, b := range parts {
		if b == ':' {
			colonIndex = i
			break
		}
	}

	if colonIndex > 0 {
		stateToken = string(parts[:colonIndex])
		sessionID = string(parts[colonIndex+1:])
	} else {
		stateToken = state
	}

	key := fmt.Sprintf("oauth:state:%s", stateToken)

	// Get and delete the state in one operation
	storedValue, err := s.client.GetDel(s.ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("invalid or expired state token")
	}
	if err != nil {
		return "", fmt.Errorf("failed to retrieve OAuth state: %w", err)
	}

	// Verify the session ID matches if provided
	if sessionID != "" && storedValue != sessionID {
		return "", fmt.Errorf("session ID mismatch")
	}

	return sessionID, nil
}

// SaveJWTToken stores JWT metadata in Redis for revocation tracking
// The token is stored with a TTL matching its expiration time
func (s *Store) SaveJWTToken(jti string, athleteID int, issuedAt time.Time, expiresAt time.Time) error {
	key := fmt.Sprintf("jwt:jti:%s", jti)

	// Calculate TTL based on expiration time
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return fmt.Errorf("token already expired")
	}

	// Store token metadata
	data := map[string]interface{}{
		"athlete_id": athleteID,
		"issued_at":  issuedAt.Unix(),
		"expires_at": expiresAt.Unix(),
	}

	err := s.client.HSet(s.ctx, key, data).Err()
	if err != nil {
		return fmt.Errorf("failed to save JWT metadata: %w", err)
	}

	// Set expiration
	err = s.client.Expire(s.ctx, key, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set JWT expiration: %w", err)
	}

	slog.Info("saved JWT token metadata", "jti", jti, "athlete_id", athleteID)
	return nil
}

// RevokeJWTToken marks a JWT token as revoked
// The revocation is stored until the token's expiration time
func (s *Store) RevokeJWTToken(jti string) error {
	// First, check if the token exists
	jwtKey := fmt.Sprintf("jwt:jti:%s", jti)
	exists, err := s.client.Exists(s.ctx, jwtKey).Result()
	if err != nil {
		return fmt.Errorf("failed to check token existence: %w", err)
	}
	if exists == 0 {
		return fmt.Errorf("token not found or already expired")
	}

	// Get the token's expiration time
	ttl, err := s.client.TTL(s.ctx, jwtKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get token TTL: %w", err)
	}

	// Mark as revoked with the same TTL
	revokeKey := fmt.Sprintf("jwt:revoked:%s", jti)
	err = s.client.Set(s.ctx, revokeKey, time.Now().Unix(), ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	slog.Info("revoked JWT token", "jti", jti)
	return nil
}

// IsJWTRevoked checks if a JWT token has been revoked
func (s *Store) IsJWTRevoked(jti string) (bool, error) {
	revokeKey := fmt.Sprintf("jwt:revoked:%s", jti)

	exists, err := s.client.Exists(s.ctx, revokeKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check revocation status: %w", err)
	}

	return exists > 0, nil
}

// SaveCLISession stores a JWT token for CLI polling with a 60-second TTL
func (s *Store) SaveCLISession(sessionID string, jwt string) error {
	key := fmt.Sprintf("cli-session:%s", sessionID)

	err := s.client.Set(s.ctx, key, jwt, 60*time.Second).Err()
	if err != nil {
		slog.Error("failed to save CLI session", "session_id", sessionID, "err", err)
		return fmt.Errorf("failed to save CLI session: %w", err)
	}

	slog.Info("saved CLI session", "session_id", sessionID)
	return nil
}

// GetCLISession retrieves a JWT token for CLI polling
func (s *Store) GetCLISession(sessionID string) (string, error) {
	key := fmt.Sprintf("cli-session:%s", sessionID)

	jwt, err := s.client.Get(s.ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("session not found or expired")
	}
	if err != nil {
		slog.Error("failed to retrieve CLI session", "session_id", sessionID, "err", err)
		return "", fmt.Errorf("failed to retrieve CLI session: %w", err)
	}

	return jwt, nil
}
