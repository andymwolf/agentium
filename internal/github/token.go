// Package github provides GitHub App authentication utilities.
package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// InstallationToken represents a GitHub App installation access token.
type InstallationToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// TokenExchanger exchanges GitHub App JWTs for installation access tokens.
type TokenExchanger struct {
	httpClient *http.Client
	baseURL    string
}

// TokenExchangerOption configures a TokenExchanger.
type TokenExchangerOption func(*TokenExchanger)

// WithHTTPClient sets a custom HTTP client for the TokenExchanger.
func WithHTTPClient(client *http.Client) TokenExchangerOption {
	return func(t *TokenExchanger) {
		t.httpClient = client
	}
}

// WithBaseURL sets a custom base URL for the GitHub API (useful for testing).
func WithBaseURL(url string) TokenExchangerOption {
	return func(t *TokenExchanger) {
		t.baseURL = url
	}
}

// NewTokenExchanger creates a new TokenExchanger with the given options.
func NewTokenExchanger(opts ...TokenExchangerOption) *TokenExchanger {
	t := &TokenExchanger{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    "https://api.github.com",
	}

	for _, opt := range opts {
		opt(t)
	}

	return t
}

// ExchangeToken exchanges a GitHub App JWT for an installation access token.
// The JWT must be valid and signed with the App's private key.
// The returned token is valid for 1 hour.
func (t *TokenExchanger) ExchangeToken(jwt string, installationID int64) (*InstallationToken, error) {
	if jwt == "" {
		return nil, fmt.Errorf("JWT cannot be empty")
	}
	if installationID <= 0 {
		return nil, fmt.Errorf("installation ID must be positive")
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", t.baseURL, installationID)

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, parseAPIError(resp.StatusCode, body)
	}

	var token InstallationToken
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &token, nil
}

// apiError represents an error response from the GitHub API.
type apiError struct {
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url"`
}

// parseAPIError parses a GitHub API error response.
func parseAPIError(statusCode int, body []byte) error {
	var apiErr apiError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return fmt.Errorf("API error (status %d): %s", statusCode, string(body))
	}

	switch statusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized: %s (check JWT validity and expiration)", apiErr.Message)
	case http.StatusForbidden:
		return fmt.Errorf("forbidden: %s (check App permissions)", apiErr.Message)
	case http.StatusNotFound:
		return fmt.Errorf("not found: %s (check installation ID)", apiErr.Message)
	default:
		return fmt.Errorf("API error (status %d): %s", statusCode, apiErr.Message)
	}
}
