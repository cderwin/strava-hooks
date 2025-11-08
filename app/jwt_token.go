package app

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenClaims represents the JWT claims for our access tokens
type TokenClaims struct {
	AthleteID int   `json:"athlete_id"`
	ExpiresAt int64 `json:"expires_at"`
	jwt.RegisteredClaims
}

// GenerateJWT creates a new JWT token for the given athlete ID
func GenerateJWT(athleteID int, secret string, expirationDuration time.Duration) (string, error) {
	now := time.Now()
	expiresAt := now.Add(expirationDuration)

	claims := TokenClaims{
		AthleteID: athleteID,
		ExpiresAt: expiresAt.Unix(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
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
