package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

const (
	authUrl     = "https://www.strava.com/oauth/authorize"
	tokenUrl    = "https://www.strava.com/oauth/token"
)

type TokenResponse struct {
	TokenType    string `json:"token_type"`
	ExpiresAt    int64  `json:"expires_at"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	Athlete      struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
	} `json:"athlete"`
}

type Server struct {
	config Config
}

func NewServer() Server {
	config := LoadConfig()
	return Server{config: config}
}

func (s *Server) RunForever() {
	http.HandleFunc("/healthcheck", handleHealthcheck)
	http.HandleFunc("/login", s.handleLogin)
	http.HandleFunc("/callback", s.handleCallback)

	log.Println("Server starting on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

type HealthcheckResponse struct {
	Ok bool `json:"ok"`
}

func handleHealthcheck(w http.ResponseWriter, req *http.Request) {
	json.NewEncoder(w).Encode(HealthcheckResponse{Ok: true})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	redirectUrl := fmt.Sprintf("%s/callback", s.config.BaseUrl)
	params := url.Values{}
	params.Add("client_id", s.config.StravaClientId)
	params.Add("redirect_uri", redirectUrl)
	params.Add("response_type", "code")
	params.Add("scope", "read,activity:read_all")

	authorizationURL := fmt.Sprintf("%s?%s", authUrl, params.Encode())
	http.Redirect(w, r, authorizationURL, http.StatusFound)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	// Get the authorization code from query params
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "No code in callback", http.StatusBadRequest)
		return
	}

	// Exchange code for access token
	token, err := exchangeCode(code, &s.config)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to exchange code: %v", err), http.StatusInternalServerError)
		return
	}

	// Display success page with token info
	html := fmt.Sprintf(`
		<html>
			<body>
				<h1>Authentication Successful!</h1>
				<p>Athlete ID: %d</p>
				<p>Username: %s</p>
				<p>Access Token: %s</p>
				<p>Refresh Token: %s</p>
				<p>Expires At: %d</p>
			</body>
		</html>
	`, token.Athlete.ID, token.Athlete.Username, token.AccessToken, token.RefreshToken, token.ExpiresAt)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, html)
}

func exchangeCode(code string, config *Config) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", config.StravaClientId)
	data.Set("client_secret", config.StravaClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")

	resp, err := http.Post(tokenUrl, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed: %s", body)
	}

	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, err
	}

	return &token, nil
}
