package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/youtubeanalytics/v2"
)

const (
	analyticsTokenPath = "data/token.json"
	writeTokenPath     = "data/token_write.json"
	clientSecretPath   = "client_secret.json"
	oauthCallbackPort  = "8089"
)

// analyticsScopes are read-only scopes for the Analytics API.
var analyticsScopes = []string{
	youtubeanalytics.YtAnalyticsReadonlyScope,
	youtubeanalytics.YtAnalyticsMonetaryReadonlyScope,
}

// writeScopes grant edit access to videos (tags, titles, descriptions, etc.).
var writeScopes = []string{"https://www.googleapis.com/auth/youtube.force-ssl"}

// getAnalyticsClient returns a read-only OAuth2 client for the Analytics API.
func getAnalyticsClient(ctx context.Context) (*http.Client, error) {
	return getOAuthClient(ctx, analyticsScopes, analyticsTokenPath)
}

// getWriteClient returns an OAuth2 client with scope to edit videos.
func getWriteClient(ctx context.Context) (*http.Client, error) {
	return getOAuthClient(ctx, writeScopes, writeTokenPath)
}

// getOAuthClient returns an authenticated HTTP client using OAuth2.
// It loads client_secret.json, checks for a saved token, and if needed
// starts a local server to handle the OAuth callback.
func getOAuthClient(ctx context.Context, scopes []string, tokenPath string) (*http.Client, error) {
	b, err := os.ReadFile(clientSecretPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w\n\nTo set up OAuth2:\n1. Go to Google Cloud Console → APIs & Services → Credentials\n2. Create an OAuth 2.0 Client ID (Desktop app)\n3. Download the JSON and save as %s", clientSecretPath, err, clientSecretPath)
	}

	config, err := google.ConfigFromJSON(b, scopes...)
	if err != nil {
		return nil, fmt.Errorf("parsing client secret: %w", err)
	}
	config.RedirectURL = "http://localhost:" + oauthCallbackPort + "/callback"

	// Try loading saved token
	token, err := loadTokenFrom(tokenPath)
	if err == nil && token.Valid() {
		return config.Client(ctx, token), nil
	}
	// Try refreshing
	if err == nil && token.RefreshToken != "" {
		src := config.TokenSource(ctx, token)
		newToken, refreshErr := src.Token()
		if refreshErr == nil {
			_ = saveTokenTo(newToken, tokenPath)
			return config.Client(ctx, newToken), nil
		}
	}

	// Need new auth flow
	token, err = doOAuthFlow(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := saveTokenTo(token, tokenPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save token: %v\n", err)
	}
	return config.Client(ctx, token), nil
}

func doOAuthFlow(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errMsg := r.URL.Query().Get("error")
			if errMsg == "" {
				errMsg = "no code in callback"
			}
			fmt.Fprintf(w, "<html><body><h1>Error</h1><p>%s</p></body></html>", errMsg)
			errCh <- fmt.Errorf("OAuth error: %s", errMsg)
			return
		}
		fmt.Fprintf(w, "<html><body><h1>Success!</h1><p>You can close this window and return to the terminal.</p></body></html>")
		codeCh <- code
	})

	listener, err := net.Listen("tcp", ":"+oauthCallbackPort)
	if err != nil {
		return nil, fmt.Errorf("starting OAuth callback server: %w", err)
	}

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	authURL := config.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Println("Opening browser for Google authorization...")
	fmt.Printf("If the browser doesn't open, visit:\n%s\n\n", authURL)
	openBrowserURL(authURL)

	// Wait for callback with 2-minute timeout
	select {
	case code := <-codeCh:
		token, err := config.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("exchanging code for token: %w", err)
		}
		return token, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(2 * time.Minute):
		return nil, fmt.Errorf("OAuth timed out after 2 minutes — did you complete authorization in the browser?")
	}
}

func loadTokenFrom(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var token oauth2.Token
	if err := json.NewDecoder(f).Decode(&token); err != nil {
		return nil, err
	}
	return &token, nil
}

func saveTokenTo(token *oauth2.Token, path string) error {
	if err := os.MkdirAll("data", 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(token)
}

func openBrowserURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start()
}
