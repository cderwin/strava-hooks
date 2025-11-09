package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExchangeCode(t *testing.T) {
	tests := []struct {
		name              string
		code              string
		responseStatus    int
		responseBody      string
		expectError       bool
		expectedAthleteID int
		expectedToken     string
	}{
		{
			name:           "successful code exchange",
			code:           "auth-code-123",
			responseStatus: http.StatusOK,
			responseBody: `{
				"token_type": "Bearer",
				"expires_at": 1609459200,
				"expires_in": 21600,
				"refresh_token": "refresh-token-abc",
				"access_token": "access-token-xyz",
				"athlete": {
					"id": 12345,
					"username": "testuser"
				}
			}`,
			expectError:       false,
			expectedAthleteID: 12345,
			expectedToken:     "access-token-xyz",
		},
		{
			name:           "invalid code",
			code:           "invalid-code",
			responseStatus: http.StatusBadRequest,
			responseBody:   `{"error": "invalid_grant"}`,
			expectError:    true,
		},
		{
			name:           "server error",
			code:           "auth-code-456",
			responseStatus: http.StatusInternalServerError,
			responseBody:   `{"error": "internal_server_error"}`,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server to mock Strava's token endpoint
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify it's a POST request
				if r.Method != "POST" {
					t.Errorf("expected POST request, got %s", r.Method)
				}

				// Verify Content-Type header
				contentType := r.Header.Get("Content-Type")
				if contentType != "application/x-www-form-urlencoded" {
					t.Errorf("expected Content-Type application/x-www-form-urlencoded, got %q", contentType)
				}

				// Parse the form data
				if err := r.ParseForm(); err != nil {
					t.Fatalf("failed to parse form: %v", err)
				}

				// Verify required fields are present
				if r.FormValue("code") != tt.code {
					t.Errorf("expected code %q, got %q", tt.code, r.FormValue("code"))
				}
				if r.FormValue("grant_type") != "authorization_code" {
					t.Errorf("expected grant_type authorization_code, got %q", r.FormValue("grant_type"))
				}
				if r.FormValue("client_id") == "" {
					t.Error("client_id should not be empty")
				}
				if r.FormValue("client_secret") == "" {
					t.Error("client_secret should not be empty")
				}

				w.WriteHeader(tt.responseStatus)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Override the tokenUrl for testing
			originalTokenUrl := tokenUrl
			defer func() { tokenUrl = originalTokenUrl }()
			tokenUrl = server.URL

			// Create config and client
			config := &Config{
				StravaClientId:     "test-client-id",
				StravaClientSecret: "test-client-secret",
			}
			client := NewStravaClient("")

			// Call exchangeCode
			tokenResponse, err := exchangeCode(tt.code, config, &client)

			// Check error expectation
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify token response if no error expected
			if !tt.expectError && tokenResponse != nil {
				if tokenResponse.Athlete.ID != tt.expectedAthleteID {
					t.Errorf("expected athlete ID %d, got %d", tt.expectedAthleteID, tokenResponse.Athlete.ID)
				}
				if tokenResponse.AccessToken != tt.expectedToken {
					t.Errorf("expected access token %q, got %q", tt.expectedToken, tokenResponse.AccessToken)
				}
				if tokenResponse.TokenType != "Bearer" {
					t.Errorf("expected token type Bearer, got %q", tokenResponse.TokenType)
				}
			}
		})
	}
}

func TestTokenResponseParsing(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		expectError bool
	}{
		{
			name: "complete token response",
			jsonData: `{
				"token_type": "Bearer",
				"expires_at": 1609459200,
				"expires_in": 21600,
				"refresh_token": "refresh-token",
				"access_token": "access-token",
				"athlete": {
					"id": 12345,
					"username": "testuser"
				}
			}`,
			expectError: false,
		},
		{
			name: "minimal token response",
			jsonData: `{
				"access_token": "access-token",
				"athlete": {
					"id": 12345
				}
			}`,
			expectError: false,
		},
		{
			name:        "invalid json",
			jsonData:    `{"invalid": json}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tokenResponse TokenResponse
			err := json.Unmarshal([]byte(tt.jsonData), &tokenResponse)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.expectError {
				if tokenResponse.AccessToken == "" {
					t.Error("access token should not be empty")
				}
				if tokenResponse.Athlete.ID == 0 {
					t.Error("athlete ID should not be zero")
				}
			}
		})
	}
}

func TestExchangeCode_FormDataEncoding(t *testing.T) {
	// Test that special characters in the config are properly URL encoded
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		// Verify special characters are properly encoded/decoded
		clientSecret := r.FormValue("client_secret")
		if clientSecret != "secret-with-special-chars!@#$%" {
			t.Errorf("expected special characters to be preserved, got %q", clientSecret)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"token_type": "Bearer",
			"access_token": "token",
			"athlete": {"id": 1}
		}`))
	}))
	defer server.Close()

	originalTokenUrl := tokenUrl
	defer func() { tokenUrl = originalTokenUrl }()
	tokenUrl = server.URL

	config := &Config{
		StravaClientId:     "test-client",
		StravaClientSecret: "secret-with-special-chars!@#$%",
	}
	client := NewStravaClient("")

	_, err := exchangeCode("test-code", config, &client)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
