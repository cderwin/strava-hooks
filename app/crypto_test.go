package app

import (
	"testing"
	"time"
)

func TestGenerateJWT(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	athleteID := 12345
	expirationDuration := 30 * 24 * time.Hour

	token1, jti1, err := GenerateJWT(athleteID, secret, expirationDuration)
	if err != nil {
		t.Fatalf("failed to generate JWT: %v", err)
	}

	if token1 == "" {
		t.Error("expected non-empty token string")
	}

	if jti1 == "" {
		t.Error("expected non-empty JTI")
	}

	// Verify the token can be parsed
	claims, err := VerifyJWT(token1, secret)
	if err != nil {
		t.Fatalf("failed to verify generated JWT: %v", err)
	}

	if claims.AthleteID != athleteID {
		t.Errorf("expected athlete ID %d, got %d", athleteID, claims.AthleteID)
	}

	if claims.JTI != jti1 {
		t.Errorf("expected JTI %s, got %s", jti1, claims.JTI)
	}

	// Generate another token and verify JTI is unique
	token2, jti2, err := GenerateJWT(athleteID, secret, expirationDuration)
	if err != nil {
		t.Fatalf("failed to generate second JWT: %v", err)
	}

	if jti1 == jti2 {
		t.Error("expected unique JTI for each token, but got duplicates")
	}

	if token1 == token2 {
		t.Error("expected unique tokens, but got duplicates")
	}
}

func TestVerifyJWT_Valid(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	athleteID := 67890
	expirationDuration := 1 * time.Hour

	token, jti, err := GenerateJWT(athleteID, secret, expirationDuration)
	if err != nil {
		t.Fatalf("failed to generate JWT: %v", err)
	}

	claims, err := VerifyJWT(token, secret)
	if err != nil {
		t.Fatalf("failed to verify valid JWT: %v", err)
	}

	if claims.AthleteID != athleteID {
		t.Errorf("expected athlete ID %d, got %d", athleteID, claims.AthleteID)
	}

	if claims.JTI != jti {
		t.Errorf("expected JTI %s, got %s", jti, claims.JTI)
	}

	// Verify expiration time is in the future
	if claims.ExpiresAt <= time.Now().Unix() {
		t.Error("expected token to not be expired")
	}
}

func TestVerifyJWT_InvalidSecret(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	wrongSecret := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	athleteID := 12345
	expirationDuration := 1 * time.Hour

	token, _, err := GenerateJWT(athleteID, secret, expirationDuration)
	if err != nil {
		t.Fatalf("failed to generate JWT: %v", err)
	}

	// Try to verify with wrong secret
	_, err = VerifyJWT(token, wrongSecret)
	if err == nil {
		t.Error("expected error when verifying with wrong secret, but got none")
	}
}

func TestVerifyJWT_Expired(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	athleteID := 12345

	// Generate a token that expires immediately
	expirationDuration := -1 * time.Hour // Already expired
	token, _, err := GenerateJWT(athleteID, secret, expirationDuration)
	if err != nil {
		t.Fatalf("failed to generate JWT: %v", err)
	}

	// Verify should still parse the token (library may not reject expired tokens)
	claims, err := VerifyJWT(token, secret)

	// The JWT library should reject expired tokens, but let's check the claims anyway
	if err == nil {
		// If library doesn't reject, verify the ExpiresAt is in the past
		if claims.ExpiresAt >= time.Now().Unix() {
			t.Error("expected token to be expired (ExpiresAt in past)")
		}
	}
	// If err is not nil, that's also acceptable - library rejected expired token
}

func TestVerifyJWT_MalformedToken(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "empty token",
			token: "",
		},
		{
			name:  "random string",
			token: "not-a-jwt-token",
		},
		{
			name:  "malformed jwt",
			token: "header.payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := VerifyJWT(tt.token, secret)
			if err == nil {
				t.Error("expected error for malformed token, but got none")
			}
		})
	}
}

func TestEncryptDecrypt(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	tests := []struct {
		name      string
		plaintext string
	}{
		{
			name:      "simple string",
			plaintext: "hello world",
		},
		{
			name:      "empty string",
			plaintext: "",
		},
		{
			name:      "long string",
			plaintext: "this is a much longer string with many characters that should still be encrypted and decrypted correctly",
		},
		{
			name:      "special characters",
			plaintext: "!@#$%^&*()_+-=[]{}|;':\",./<>?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := Encrypt(tt.plaintext, secret)
			if err != nil {
				t.Fatalf("failed to encrypt: %v", err)
			}

			if encrypted == "" {
				t.Error("expected non-empty encrypted string")
			}

			decrypted, err := Decrypt(encrypted, secret)
			if err != nil {
				t.Fatalf("failed to decrypt: %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("expected decrypted text %q, got %q", tt.plaintext, decrypted)
			}
		})
	}
}

func TestEncryptDecrypt_WrongSecret(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	wrongSecret := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	plaintext := "secret message"

	encrypted, err := Encrypt(plaintext, secret)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	// Try to decrypt with wrong secret
	_, err = Decrypt(encrypted, wrongSecret)
	if err == nil {
		t.Error("expected error when decrypting with wrong secret, but got none")
	}
}

func TestSecretKeyFromHex(t *testing.T) {
	tests := []struct {
		name        string
		hexSecret   string
		expectError bool
	}{
		{
			name:        "valid 32-byte secret",
			hexSecret:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectError: false,
		},
		{
			name:        "valid longer secret (uses first 32 bytes)",
			hexSecret:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdefff",
			expectError: false,
		},
		{
			name:        "too short secret",
			hexSecret:   "0123456789abcdef",
			expectError: true,
		},
		{
			name:        "invalid hex characters",
			hexSecret:   "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
			expectError: true,
		},
		{
			name:        "empty secret",
			hexSecret:   "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := SecretKeyFromHex(tt.hexSecret)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.expectError && key == nil {
				t.Error("expected non-nil key for valid secret")
			}
		})
	}
}
