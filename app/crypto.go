package app

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/nacl/secretbox"
)

// TokenClaims represents the JWT claims for our access tokens
type TokenClaims struct {
	AthleteID int    `json:"athlete_id"`
	ExpiresAt int64  `json:"expires_at"`
	JTI       string `json:"jti"` // JWT ID for revocation tracking
	jwt.RegisteredClaims
}

// generateJTI creates a random JWT ID for token revocation tracking
func generateJTI() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GenerateJWT creates a new JWT token for the given athlete ID
// Returns the token string and the unique JWT ID (jti)
func GenerateJWT(athleteID int, secret string, expirationDuration time.Duration) (string, string, error) {
	now := time.Now()
	expiresAt := now.Add(expirationDuration)
	jti := generateJTI()

	claims := TokenClaims{
		AthleteID: athleteID,
		ExpiresAt: expiresAt.Unix(),
		JTI:       jti,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", "", err
	}
	return tokenString, jti, nil
}

// VerifyJWT validates a JWT token and returns the claims
func VerifyJWT(tokenString string, secret string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*TokenClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// SecretKeyFromHex converts a hex-encoded string to a 32-byte secret key.
// Returns an error if the decoded secret is less than 32 bytes.
// If longer than 32 bytes, only the first 32 bytes are used.
func SecretKeyFromHex(hexSecret string) (*[32]byte, error) {
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

// Encrypt encrypts a string using NaCl secretbox with the provided hex-encoded secret.
// The secret must be at least 32 bytes when decoded from hex.
// Returns base64-encoded ciphertext.
func Encrypt(plaintext string, hexSecret string) (string, error) {
	// Generate a random nonce
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	secretBytes, err := SecretKeyFromHex(hexSecret)
	if err != nil {
		return "", fmt.Errorf("failed to decode secret: %w", err)
	}

	// Encrypt the plaintext
	encrypted := secretbox.Seal(nonce[:], []byte(plaintext), &nonce, secretBytes)

	// Encode to base64 for easy storage/transmission
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// Decrypt decrypts a base64-encoded encrypted string using NaCl secretbox.
// The secret must be at least 32 bytes when decoded from hex.
// Returns the original plaintext string.
func Decrypt(ciphertext string, hexSecret string) (string, error) {
	// Decode from base64
	encrypted, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// Extract nonce (first 24 bytes)
	if len(encrypted) < 24 {
		return "", errors.New("ciphertext too short")
	}

	var nonce [24]byte
	copy(nonce[:], encrypted[:24])

	secretBytes, err := SecretKeyFromHex(hexSecret)
	if err != nil {
		return "", fmt.Errorf("failed to decode secret: %w", err)
	}

	// Decrypt the ciphertext
	decrypted, ok := secretbox.Open(nil, encrypted[24:], &nonce, secretBytes)
	if !ok {
		return "", errors.New("decryption failed: invalid secret or corrupted data")
	}

	return string(decrypted), nil
}
