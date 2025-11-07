package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/crypto/nacl/secretbox"

	"github.com/redis/go-redis/v9"
)

type Store struct {
	client *redis.Client
	ctx    context.Context
	config *Config
}

type TokenInfo struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int
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

	encryptedAccessToken, err := encryptToken(token.AccessToken, s.config.Secret)
	if err != nil {
		return fmt.Errorf("failed to encrypt access token: %w", err)
	}

	encryptedRefreshToken, err := encryptToken(token.RefreshToken, s.config.Secret)
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

	tokenInfo.AccessToken, err = decryptToken(tokenInfo.AccessToken, s.config.Secret)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt access token: %w", err)
	}

	tokenInfo.RefreshToken, err = decryptToken(tokenInfo.RefreshToken, s.config.Secret)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret token: %w", err)
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
	queryParams := url.Values{}
	queryParams.Add("client_id", s.config.StravaClientId)
	queryParams.Add("client_secret", s.config.StravaClientSecret)
	queryParams.Add("grant_type", "refresh_token")
	queryParams.Add("refresh_token", Token.RefreshToken)
	resp, err := http.PostForm(tokenUrl, queryParams)
	if err != nil {
		slog.Error("error refreshing token", "err", err)
		return nil, err
	}

	var newToken TokenInfo
	err = json.NewDecoder(resp.Body).Decode(&newToken)
	if err != nil {
		slog.Error("error decoding refresh token response", "err", err)
		return nil, err
	}

	s.SaveToken(AthleteId, newToken)
	return &newToken, nil
}

// SecretFromHex converts a hex-encoded string to a 32-byte secret key.
// Returns an error if the decoded secret is less than 32 bytes.
// If longer than 32 bytes, only the first 32 bytes are used.
func secretFromHex(hexSecret string) (*[32]byte, error) {
	decoded, err := hex.DecodeString(hexSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hex string: %w", err)
	}

	if len(decoded) < 32 {
		return nil, fmt.Errorf("secret must be at least 32 bytes, got %d bytes", len(decoded))
	}

	var secret [32]byte
	copy(secret[:], decoded[:32])
	return &secret, nil
}

// EncryptToken encrypts a token using NaCl secretbox with the provided secret.
// The secret must be exactly 32 bytes. Returns base64-encoded ciphertext.
func encryptToken(token string, secret string) (string, error) {
	// Generate a random nonce
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	secretBytes, err := secretFromHex(secret)
	if err != nil {
		return "", fmt.Errorf("failed to decode secret: %w", err)
	}

	// Encrypt the token
	encrypted := secretbox.Seal(nonce[:], []byte(token), &nonce, secretBytes)

	// Encode to base64 for easy storage/transmission
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// DecryptToken decrypts a base64-encoded encrypted token using NaCl secretbox.
// The secret must be exactly 32 bytes. Returns the original token string.
func decryptToken(encryptedToken string, secret string) (string, error) {
	// Decode from base64
	encrypted, err := base64.StdEncoding.DecodeString(encryptedToken)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// Extract nonce (first 24 bytes)
	if len(encrypted) < 24 {
		return "", errors.New("ciphertext too short")
	}

	var nonce [24]byte
	copy(nonce[:], encrypted[:24])

	secretBytes, err := secretFromHex(secret)
	if err != nil {
		return "", fmt.Errorf("failed to decode secret: %w", err)
	}

	// Decrypt the token
	decrypted, ok := secretbox.Open(nil, encrypted[24:], &nonce, secretBytes)
	if !ok {
		return "", errors.New("decryption failed: invalid secret or corrupted data")
	}

	return string(decrypted), nil
}
