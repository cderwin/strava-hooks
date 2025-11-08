package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type OAuthState struct {
	client *redis.Client
	ctx    context.Context
}

// generateStateToken creates a random state token
func generateStateToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// SaveState stores the challenge code with a state token in Redis
// Returns the state token
func (os *OAuthState) SaveState(challenge string) (string, error) {
	state := generateStateToken()
	key := fmt.Sprintf("oauth:state:%s", state)

	// Store the challenge with a 10-minute expiration
	err := os.client.Set(os.ctx, key, challenge, 10*time.Minute).Err()
	if err != nil {
		return "", fmt.Errorf("failed to save OAuth state: %w", err)
	}

	return state, nil
}

// GetState retrieves and deletes the challenge code for a given state token
func (os *OAuthState) GetState(state string) (string, error) {
	key := fmt.Sprintf("oauth:state:%s", state)

	// Get and delete the state in one operation
	challenge, err := os.client.GetDel(os.ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("invalid or expired state token")
	}
	if err != nil {
		return "", fmt.Errorf("failed to retrieve OAuth state: %w", err)
	}

	return challenge, nil
}
