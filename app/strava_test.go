package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStravaClient_performRequest(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		method         string
		responseStatus int
		responseBody   string
		expectError    bool
	}{
		{
			name:           "successful GET request with token",
			token:          "test-token-123",
			method:         "GET",
			responseStatus: http.StatusOK,
			responseBody:   `{"id": 123}`,
			expectError:    false,
		},
		{
			name:           "successful GET request without token",
			token:          "",
			method:         "GET",
			responseStatus: http.StatusOK,
			responseBody:   `{"success": true}`,
			expectError:    false,
		},
		{
			name:           "failed request with 4xx status",
			token:          "test-token-123",
			method:         "GET",
			responseStatus: http.StatusBadRequest,
			responseBody:   `{"error": "bad request"}`,
			expectError:    true,
		},
		{
			name:           "failed request with 5xx status",
			token:          "test-token-123",
			method:         "GET",
			responseStatus: http.StatusInternalServerError,
			responseBody:   `{"error": "server error"}`,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify the Authorization header if token is provided
				if tt.token != "" {
					authHeader := r.Header.Get("Authorization")
					expectedAuth := "Bearer " + tt.token
					if authHeader != expectedAuth {
						t.Errorf("expected Authorization header %q, got %q", expectedAuth, authHeader)
					}
				}

				// Verify the method
				if r.Method != tt.method {
					t.Errorf("expected method %s, got %s", tt.method, r.Method)
				}

				w.WriteHeader(tt.responseStatus)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create client and make request
			client := NewStravaClient(tt.token)
			body, err := client.performRequest(tt.method, server.URL, nil)

			// Check error expectation
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify response body if no error expected
			if !tt.expectError && body != nil {
				responseBytes, _ := io.ReadAll(body)
				if string(responseBytes) != tt.responseBody {
					t.Errorf("expected body %q, got %q", tt.responseBody, string(responseBytes))
				}
			}
		})
	}
}

func TestStravaClient_performRequestWithHeaders(t *testing.T) {
	tests := []struct {
		name            string
		token           string
		customHeaders   map[string]string
		expectedHeaders map[string]string
	}{
		{
			name:  "request with custom headers and token",
			token: "test-token",
			customHeaders: map[string]string{
				"Content-Type": "application/json",
				"X-Custom":     "custom-value",
			},
			expectedHeaders: map[string]string{
				"Authorization": "Bearer test-token",
				"Content-Type":  "application/json",
				"X-Custom":      "custom-value",
			},
		},
		{
			name:  "request with custom headers without token",
			token: "",
			customHeaders: map[string]string{
				"Content-Type": "application/x-www-form-urlencoded",
			},
			expectedHeaders: map[string]string{
				"Content-Type": "application/x-www-form-urlencoded",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify all expected headers are present
				for key, expectedValue := range tt.expectedHeaders {
					actualValue := r.Header.Get(key)
					if actualValue != expectedValue {
						t.Errorf("expected header %s=%q, got %q", key, expectedValue, actualValue)
					}
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))
			defer server.Close()

			client := NewStravaClient(tt.token)
			_, err := client.performRequestWithHeaders("GET", server.URL, nil, tt.customHeaders)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestStravaClient_performRequestForm(t *testing.T) {
	tests := []struct {
		name         string
		formData     map[string]string
		expectedBody map[string]string
	}{
		{
			name: "simple form data",
			formData: map[string]string{
				"client_id":     "12345",
				"client_secret": "secret",
				"grant_type":    "authorization_code",
			},
			expectedBody: map[string]string{
				"client_id":     "12345",
				"client_secret": "secret",
				"grant_type":    "authorization_code",
			},
		},
		{
			name: "form data with special characters",
			formData: map[string]string{
				"code":         "abc123!@#",
				"redirect_uri": "https://example.com/callback?foo=bar",
			},
			expectedBody: map[string]string{
				"code":         "abc123!@#",
				"redirect_uri": "https://example.com/callback?foo=bar",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify Content-Type header
				contentType := r.Header.Get("Content-Type")
				if contentType != "application/x-www-form-urlencoded" {
					t.Errorf("expected Content-Type application/x-www-form-urlencoded, got %q", contentType)
				}

				// Parse form data
				if err := r.ParseForm(); err != nil {
					t.Fatalf("failed to parse form: %v", err)
				}

				// Verify all form fields
				for key, expectedValue := range tt.expectedBody {
					actualValue := r.FormValue(key)
					if actualValue != expectedValue {
						t.Errorf("expected form field %s=%q, got %q", key, expectedValue, actualValue)
					}
				}

				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"success": true}`))
			}))
			defer server.Close()

			client := NewStravaClient("")
			body, err := client.performRequestForm("POST", server.URL, tt.formData)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify we can read the response
			if body != nil {
				responseBytes, _ := io.ReadAll(body)
				if !strings.Contains(string(responseBytes), "success") {
					t.Errorf("unexpected response body: %s", string(responseBytes))
				}
			}
		})
	}
}

func TestStravaClient_GetActivity(t *testing.T) {
	tests := []struct {
		name           string
		activityID     string
		responseStatus int
		responseBody   string
		expectError    bool
		expectedName   string
	}{
		{
			name:           "successful activity fetch",
			activityID:     "12345",
			responseStatus: http.StatusOK,
			responseBody:   `{"id": 12345, "name": "Morning Run", "distance": 5000}`,
			expectError:    false,
			expectedName:   "Morning Run",
		},
		{
			name:           "activity not found",
			activityID:     "99999",
			responseStatus: http.StatusNotFound,
			responseBody:   `{"error": "not found"}`,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify the URL contains the activity ID
				if !strings.Contains(r.URL.Path, tt.activityID) {
					t.Errorf("expected URL to contain activity ID %s, got %s", tt.activityID, r.URL.Path)
				}

				w.WriteHeader(tt.responseStatus)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Override the ActivityUrl constant for testing
			originalActivityUrl := ActivityUrl
			defer func() { ActivityUrl = originalActivityUrl }()
			ActivityUrl = server.URL + "/activities/%s"

			client := NewStravaClient("test-token")
			activity, err := client.GetActivity(tt.activityID)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.expectError && activity.Name != tt.expectedName {
				t.Errorf("expected activity name %q, got %q", tt.expectedName, activity.Name)
			}
		})
	}
}

func TestNewStravaClient(t *testing.T) {
	token := "test-token-123"
	client := NewStravaClient(token)

	if client.Token != token {
		t.Errorf("expected token %q, got %q", token, client.Token)
	}

	// Verify the client has an http.Client
	if client.client.Timeout != 0 {
		// Just checking that the client field exists and is initialized
	}
}
