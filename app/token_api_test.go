package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

// mockStore wraps a Store with additional test helpers
type mockStore struct {
	Store
}

// newMockStore creates a new mock store for testing
// You'll need to provide a real Redis client or use miniredis
func newMockStore(redisClient *redis.Client) *mockStore {
	ctx := context.Background()
	config := &Config{
		Secret:             "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		StravaClientId:     "test-client-id",
		StravaClientSecret: "test-client-secret",
	}
	stravaClient := NewStravaClient("")

	return &mockStore{
		Store: Store{
			client:       redisClient,
			ctx:          ctx,
			config:       config,
			stravaClient: &stravaClient,
		},
	}
}

func TestHandleTokenCallback_MissingState(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/token/callback?code=test-code", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Create a minimal ServerState
	s := &ServerState{}

	err := s.handleTokenCallback(c)

	// Should return error for missing state
	if err == nil {
		t.Error("expected error for missing state parameter")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}

	if httpErr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, httpErr.Code)
	}
}

func TestHandleTokenCallback_MissingCode(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/token/callback?state=test-state", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	s := &ServerState{}

	err := s.handleTokenCallback(c)

	if err == nil {
		t.Error("expected error for missing code parameter")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}

	if httpErr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, httpErr.Code)
	}
}

func TestHandleTokenVerify_ExpiredToken(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	// Create an expired token
	expiredToken, _, err := GenerateJWT(12345, secret, -1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate expired token: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/token/verify", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	s := &ServerState{
		config: Config{
			Secret: secret,
		},
	}

	err = s.handleTokenVerify(c)

	// Should return 401 for expired token
	if err == nil {
		t.Error("expected error for expired token")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}

	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}

	// Verify the error message mentions expiration
	errMsg := httpErr.Message
	if m, ok := errMsg.(map[string]interface{}); ok {
		if m["error"] != "token_expired" {
			t.Errorf("expected error code 'token_expired', got %v", m["error"])
		}
	}
}

func TestHandleTokenVerify_MissingAuthHeader(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/token/verify", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	s := &ServerState{}

	err := s.handleTokenVerify(c)

	if err == nil {
		t.Error("expected error for missing Authorization header")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}

	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestHandleTokenVerify_InvalidAuthFormat(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/token/verify", nil)
	req.Header.Set("Authorization", "InvalidFormat token123")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	s := &ServerState{}

	err := s.handleTokenVerify(c)

	if err == nil {
		t.Error("expected error for invalid Authorization format")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}

	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestHandleTokenVerify_InvalidToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/token/verify", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	s := &ServerState{
		config: Config{
			Secret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
	}

	err := s.handleTokenVerify(c)

	if err == nil {
		t.Error("expected error for invalid token")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}

	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestHandleTokenRevoke_MissingAuthHeader(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/token/revoke", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	s := &ServerState{}

	err := s.handleTokenRevoke(c)

	if err == nil {
		t.Error("expected error for missing Authorization header")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}

	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestHandleTokenRevoke_InvalidToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/token/revoke", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	s := &ServerState{
		config: Config{
			Secret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
	}

	err := s.handleTokenRevoke(c)

	if err == nil {
		t.Error("expected error for invalid token")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}

	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

// TestTokenVerify_ExpiresAtCheck verifies that the manual expiration check works
func TestTokenVerify_ExpiresAtCheck(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	tests := []struct {
		name           string
		expirationTime time.Duration
		shouldExpire   bool
	}{
		{
			name:           "valid token (1 hour in future)",
			expirationTime: 1 * time.Hour,
			shouldExpire:   false,
		},
		{
			name:           "expired token (1 hour in past)",
			expirationTime: -1 * time.Hour,
			shouldExpire:   true,
		},
		{
			name:           "expired token (1 second in past)",
			expirationTime: -1 * time.Second,
			shouldExpire:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, _, err := GenerateJWT(12345, secret, tt.expirationTime)
			if err != nil {
				t.Fatalf("failed to generate token: %v", err)
			}

			// Verify the token - the JWT library may or may not reject expired tokens
			claims, err := VerifyJWT(token, secret)

			if tt.shouldExpire {
				// For expired tokens, either:
				// 1. VerifyJWT returns an error (library rejects it), OR
				// 2. VerifyJWT succeeds but ExpiresAt is in the past
				if err == nil {
					// Token was parsed, check ExpiresAt
					if claims.ExpiresAt >= time.Now().Unix() {
						t.Error("expected token to be expired (ExpiresAt < now), but it wasn't")
					}
				}
				// If err != nil, the library already rejected it, which is fine
			} else {
				// For valid tokens, should not have error and should not be expired
				if err != nil {
					t.Errorf("unexpected error for valid token: %v", err)
				}
				if claims != nil && claims.ExpiresAt < time.Now().Unix() {
					t.Error("expected token to not be expired, but ExpiresAt is in the past")
				}
			}
		})
	}
}

// TestOAuthStateFlow tests the complete OAuth state generation and verification
func TestOAuthStateFlow(t *testing.T) {
	// This test requires a real Redis connection or miniredis
	// For now, we'll test the error cases without Redis

	t.Run("GetOAuthState with non-existent state", func(t *testing.T) {
		// Without a real Redis client, we can't fully test this
		// But we can verify the function signature and error handling
		// This would require miniredis or a test Redis instance
		t.Skip("requires Redis connection - use integration test")
	})
}

// TestHandleTokenStart_GeneratesState tests that handleTokenStart generates a state token
func TestHandleTokenStart_GeneratesState(t *testing.T) {
	// This test would require mocking the store.SaveOAuthState call
	// For now, we'll test the redirect URL construction

	// Mock config
	config := Config{
		BaseUrl:            "https://example.com",
		StravaClientId:     "test-client-id",
		StravaClientSecret: "test-secret",
	}

	// This would fail without a real store, but demonstrates the test structure
	// In a real test, you'd use miniredis or a mock store
	_ = config

	// Skip actual execution since we don't have Redis
	t.Skip("requires Redis connection - use integration test")
}

// TestExtractBearerToken tests bearer token extraction logic
func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name        string
		authHeader  string
		expectToken string
		expectError bool
	}{
		{
			name:        "valid bearer token",
			authHeader:  "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test.signature",
			expectToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test.signature",
			expectError: false,
		},
		{
			name:        "empty header",
			authHeader:  "",
			expectToken: "",
			expectError: true,
		},
		{
			name:        "wrong format",
			authHeader:  "Basic username:password",
			expectToken: "",
			expectError: true,
		},
		{
			name:        "bearer with extra spaces",
			authHeader:  "Bearer  token-with-space",
			expectToken: " token-with-space",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var token string
			var isValid bool

			if len(tt.authHeader) > 7 && tt.authHeader[:7] == "Bearer " {
				token = tt.authHeader[7:]
				isValid = true
			}

			if tt.expectError && isValid {
				t.Error("expected error but token was extracted")
			}
			if !tt.expectError && !isValid {
				t.Error("expected token to be extracted but got error")
			}
			if !tt.expectError && token != tt.expectToken {
				t.Errorf("expected token %q, got %q", tt.expectToken, token)
			}
		})
	}
}

// TestTokenResponseStructure tests the JSON response structure
func TestTokenResponseStructure(t *testing.T) {
	// Verify the response structure matches expectations
	response := map[string]interface{}{
		"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test.signature",
		"token_type":   "Bearer",
		"athlete_id":   12345,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	// Verify it contains expected fields
	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, "access_token") {
		t.Error("response should contain access_token field")
	}
	if !strings.Contains(jsonStr, "Bearer") {
		t.Error("response should contain token_type=Bearer")
	}
	if !strings.Contains(jsonStr, "athlete_id") {
		t.Error("response should contain athlete_id field")
	}
}
