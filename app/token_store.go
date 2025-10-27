package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/redis/go-redis/v9"
)

type TokenStore struct {
	client *redis.Client
	ctx    context.Context
	config *Config
}

type TokenInfo struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int
}

func (ts *TokenStore) refreshToken(AthleteId int, Token TokenInfo) (*TokenInfo, error) {
	queryParams := url.Values{}
	queryParams.Add("client_id", ts.config.StravaClientId)
	queryParams.Add("client_secret", ts.config.StravaClientSecret)
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

	ts.SaveToken(AthleteId, newToken)
	return &newToken, nil
}

func (ts *TokenStore) fetchTokenInfo(AthleteId int) (*TokenInfo, error) {
	authKey := fmt.Sprintf("athlete:%d:strava-token", AthleteId)
	var tokenInfo TokenInfo
	err := ts.client.HMGet(ts.ctx, authKey, "access_token", "refresh_token", "expires_at").Scan(&tokenInfo)
	if err != nil {
		if err == redis.Nil {
			slog.Error("fetch token error: athlete not found", "athlete_id", AthleteId)
			return nil, err
		}

		slog.Error("fetch token error: redis request failed", "err", err)
		return nil, err
	}

	if int64(tokenInfo.ExpiresAt) < time.Now().Unix() {
		slog.Info("token expired, refreshing token", "athlete_id", AthleteId)
		newTokenInfo, err := ts.refreshToken(AthleteId, tokenInfo)
		if err != nil {
			return nil, err
		}
		return newTokenInfo, nil
	}

	return &tokenInfo, nil
}

func (ts *TokenStore) FetchToken(AthleteId int) (string, error) {
	tokenInfo, err := ts.fetchTokenInfo(AthleteId)
	if err != nil {
		return "", err
	}

	return tokenInfo.AccessToken, nil
}

func (ts *TokenStore) SaveToken(AthleteId int, Token TokenInfo) error {
	authKey := fmt.Sprintf("athlete:%d:strava-token", AthleteId)
	expiresAtString := fmt.Sprintf("%d", Token.ExpiresAt)
	err := ts.client.HSet(ts.ctx, authKey, "access_token", Token.AccessToken, "refresh_token", Token.RefreshToken, "expires_at", expiresAtString).Err()
	if err != nil {
		slog.Error("error saving token", "err", err)
		return err
	}

	slog.Info("saved new token", "athlete_id", AthleteId)
	return nil
}
