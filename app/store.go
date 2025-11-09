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
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
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
		return fmt.Errorf("failed to encrypt access token: %w", err)
	}

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

// SaveOAuthState stores the challenge code with a state token in Redis
// Returns the state token
func (s *Store) SaveOAuthState(challenge string) (string, error) {
	state := generateStateToken()
	key := fmt.Sprintf("oauth:state:%s", state)

	// Store the challenge with a 10-minute expiration
	err := s.client.Set(s.ctx, key, challenge, 10*time.Minute).Err()
	if err != nil {
		return "", fmt.Errorf("failed to save OAuth state: %w", err)
	}

	return state, nil
}

// GetOAuthState retrieves and deletes the challenge code for a given state token
func (s *Store) GetOAuthState(state string) (string, error) {
	key := fmt.Sprintf("oauth:state:%s", state)

	// Get and delete the state in one operation
	challenge, err := s.client.GetDel(s.ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("invalid or expired state token")
	}
	if err != nil {
		return "", fmt.Errorf("failed to retrieve OAuth state: %w", err)
	}

	return challenge, nil
}
