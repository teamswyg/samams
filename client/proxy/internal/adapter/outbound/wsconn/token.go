package wsconn

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// TokenData holds the access + refresh token pair.
type TokenData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"` // unix seconds
	ServerURL    string `json:"server_url"`
}

// IsExpired returns true if the access token has expired.
func (t *TokenData) IsExpired() bool {
	return time.Now().Unix() >= t.ExpiresAt
}

// ExpiresIn returns duration until expiry.
func (t *TokenData) ExpiresIn() time.Duration {
	return time.Until(time.Unix(t.ExpiresAt, 0))
}

// tokenFilePath returns ~/.samams/token.json
func tokenFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".samams", "token.json")
}

// LoadTokenData reads token from env vars or ~/.samams/token.json.
func LoadTokenData() (*TokenData, error) {
	// 1. Environment variables (highest priority)
	if t := os.Getenv("SAMAMS_TOKEN"); t != "" {
		return &TokenData{
			AccessToken:  t,
			RefreshToken: os.Getenv("SAMAMS_REFRESH_TOKEN"),
			ExpiresAt:    time.Now().Add(8 * time.Hour).Unix(), // assume fresh when from env
			ServerURL:    os.Getenv("SAMAMS_SERVER_URL"),
		}, nil
	}

	// 2. Token file
	path := tokenFilePath()
	if path == "" {
		return nil, fmt.Errorf("cannot determine home directory")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read token file: %w", err)
	}

	var td TokenData
	if err := json.Unmarshal(data, &td); err != nil {
		// Fallback: plain text token (legacy format)
		td.AccessToken = strings.TrimSpace(string(data))
	}

	if td.AccessToken == "" {
		return nil, fmt.Errorf("empty token")
	}
	return &td, nil
}

// SaveTokenData writes token data to ~/.samams/token.json.
func SaveTokenData(td *TokenData) error {
	path := tokenFilePath()
	if path == "" {
		return fmt.Errorf("cannot determine home directory")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(td, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// ValidateToken checks if the access token is still valid on the server.
// Returns false if the server rejects it (401) or is unreachable.
func ValidateToken(serverURL, accessToken string) bool {
	// Local mode: serverURL is ws://localhost:3000 — convert to http:// for REST call.
	httpURL := strings.Replace(serverURL, "ws://", "http://", 1)
	req, err := http.NewRequest("GET", httpURL+"/user/me", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// RefreshAccessToken calls POST /user/auth/refresh to get new tokens.
func RefreshAccessToken(serverURL, refreshToken string) (*TokenData, error) {
	// Convert WebSocket URL to HTTP for REST call.
	// Local mode: serverURL is ws://localhost:3000 — convert to http:// for REST call.
	httpURL := strings.Replace(serverURL, "ws://", "http://", 1)

	body, _ := json.Marshal(map[string]string{"refresh_token": refreshToken})

	resp, err := http.Post(httpURL+"/user/auth/refresh", "application/json",
		strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresAt    int64  `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode refresh response: %w", err)
	}

	td := &TokenData{
		AccessToken:  result.Data.AccessToken,
		RefreshToken: result.Data.RefreshToken,
		ExpiresAt:    result.Data.ExpiresAt,
		ServerURL:    serverURL,
	}
	return td, nil
}

// BrowserLogin opens the browser for login and waits for the callback with tokens.
// frontendURL: e.g., "http://localhost:5173"
// Blocks until tokens are received or timeout (5 minutes).
func BrowserLogin(frontendURL string) (*TokenData, error) {
	// Start temporary callback server on random port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for callback: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/auth/callback", port)
	loginURL := fmt.Sprintf("%s/login?proxy_callback=%s", frontendURL, callbackURL)

	tokenCh := make(chan *TokenData, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		// Support both GET query params (legacy) and POST form body (secure).
		accessToken := r.FormValue("access_token")
		refreshToken := r.FormValue("refresh_token")
		expiresAtStr := r.FormValue("expires_at")

		if accessToken == "" {
			http.Error(w, "missing access_token", http.StatusBadRequest)
			errCh <- fmt.Errorf("callback missing access_token")
			return
		}

		expiresAt, _ := strconv.ParseInt(expiresAtStr, 10, 64)

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body style="font-family:sans-serif;text-align:center;padding:60px">
			<h2>Login successful!</h2>
			<p>You can close this tab and return to the terminal.</p>
			<a href="http://localhost:5173/" style="display:inline-block;margin-top:20px;padding:12px 32px;background:#00F5A0;color:#0A0E1A;font-weight:700;border-radius:8px;text-decoration:none;font-size:15px">Go to Dashboard</a>
			</body></html>`)

		tokenCh <- &TokenData{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresAt:    expiresAt,
		}
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)
	defer srv.Close()

	// Open browser.
	log.Printf("[auth] Opening browser for login: %s", loginURL)
	log.Printf("[auth] If the browser doesn't open, visit this URL manually:")
	log.Printf("[auth] %s", loginURL)
	openBrowser(loginURL)

	// Wait for callback or timeout.
	select {
	case td := <-tokenCh:
		return td, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("login timeout (5 minutes)")
	}
}

// StartAutoRefresh runs a goroutine that refreshes the token before expiry.
// onRefresh is called with the new access token when refresh succeeds.
func StartAutoRefresh(ctx context.Context, serverURL string, data *TokenData, onRefresh func(string)) {
	for {
		// Calculate when to refresh (30 minutes before expiry, minimum 1 minute).
		untilExpiry := data.ExpiresIn()
		refreshIn := untilExpiry - 30*time.Minute
		if refreshIn < time.Minute {
			refreshIn = time.Minute
		}

		log.Printf("[auth] Token expires in %s, next refresh in %s", untilExpiry.Round(time.Second), refreshIn.Round(time.Second))

		select {
		case <-ctx.Done():
			return
		case <-time.After(refreshIn):
		}

		// Attempt refresh.
		newData, err := RefreshAccessToken(serverURL, data.RefreshToken)
		if err != nil {
			log.Printf("[auth] Token refresh failed: %v", err)
			// Try again in 1 minute.
			data.ExpiresAt = time.Now().Add(time.Minute).Unix()
			continue
		}

		newData.ServerURL = serverURL
		data = newData

		// Persist new tokens.
		if err := SaveTokenData(newData); err != nil {
			log.Printf("[auth] Failed to save refreshed token: %v", err)
		}

		log.Printf("[auth] Token refreshed, new expiry: %s", time.Unix(newData.ExpiresAt, 0).Format(time.RFC3339))

		if onRefresh != nil {
			onRefresh(newData.AccessToken)
		}
	}
}

// openBrowser opens a URL in the default browser (cross-platform).
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("[auth] Failed to open browser: %v", err)
	}
}

// LoadToken is a legacy compatibility wrapper. Returns access token string.
func LoadToken() string {
	td, err := LoadTokenData()
	if err != nil {
		return ""
	}
	return td.AccessToken
}
