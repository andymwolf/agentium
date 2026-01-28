// Package github provides GitHub App authentication utilities.
package github

import (
	"fmt"
	"sync"
	"time"
)

// TokenRefreshBuffer is the time before token expiration when a refresh should be triggered.
// A 5-minute buffer ensures the token is refreshed well before the 1-hour expiry.
const TokenRefreshBuffer = 5 * time.Minute

// TokenManager manages GitHub App installation tokens with automatic refresh.
// It tracks token expiration and provides fresh tokens when needed.
type TokenManager struct {
	mu sync.RWMutex

	// Credentials for token generation
	appID          string
	installationID int64
	privateKey     []byte

	// Current token state
	token     string
	expiresAt time.Time

	// Dependencies (can be overridden for testing)
	jwtGenerator   *JWTGenerator
	tokenExchanger *TokenExchanger

	// For testing: allow time to be mocked
	nowFunc func() time.Time
}

// TokenManagerOption configures a TokenManager.
type TokenManagerOption func(*TokenManager)

// WithNowFunc sets a custom time function for testing.
func WithNowFunc(fn func() time.Time) TokenManagerOption {
	return func(tm *TokenManager) {
		tm.nowFunc = fn
	}
}

// WithTokenExchanger sets a custom token exchanger (useful for testing).
func WithTokenExchanger(exchanger *TokenExchanger) TokenManagerOption {
	return func(tm *TokenManager) {
		tm.tokenExchanger = exchanger
	}
}

// NewTokenManager creates a new TokenManager with the given GitHub App credentials.
func NewTokenManager(appID string, installationID int64, privateKey []byte, opts ...TokenManagerOption) (*TokenManager, error) {
	if appID == "" {
		return nil, fmt.Errorf("app ID cannot be empty")
	}
	if installationID <= 0 {
		return nil, fmt.Errorf("installation ID must be positive")
	}
	if len(privateKey) == 0 {
		return nil, fmt.Errorf("private key cannot be empty")
	}

	// Create JWT generator to validate private key early
	jwtGen, err := NewJWTGenerator(appID, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWT generator: %w", err)
	}

	tm := &TokenManager{
		appID:          appID,
		installationID: installationID,
		privateKey:     privateKey,
		jwtGenerator:   jwtGen,
		tokenExchanger: NewTokenExchanger(),
		nowFunc:        time.Now,
	}

	for _, opt := range opts {
		opt(tm)
	}

	return tm, nil
}

// Token returns a valid installation token, refreshing if necessary.
// This is the primary method for obtaining a token.
func (tm *TokenManager) Token() (string, error) {
	tm.mu.RLock()
	if tm.isValidLocked() {
		token := tm.token
		tm.mu.RUnlock()
		return token, nil
	}
	tm.mu.RUnlock()

	// Need to refresh
	return tm.Refresh()
}

// Refresh forces a token refresh regardless of current token validity.
// Returns the new token on success.
func (tm *TokenManager) Refresh() (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Generate a new JWT
	jwt, err := tm.jwtGenerator.GenerateToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Exchange JWT for installation token
	installToken, err := tm.tokenExchanger.ExchangeToken(jwt, tm.installationID)
	if err != nil {
		return "", fmt.Errorf("failed to exchange token: %w", err)
	}

	tm.token = installToken.Token
	tm.expiresAt = installToken.ExpiresAt

	return tm.token, nil
}

// NeedsRefresh returns true if the token is missing, expired, or will expire soon.
func (tm *TokenManager) NeedsRefresh() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return !tm.isValidLocked()
}

// ExpiresAt returns the expiration time of the current token.
// Returns zero time if no token has been fetched.
func (tm *TokenManager) ExpiresAt() time.Time {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.expiresAt
}

// isValidLocked checks if the current token is valid (must hold at least RLock).
// A token is considered invalid if it doesn't exist or will expire within the refresh buffer.
func (tm *TokenManager) isValidLocked() bool {
	if tm.token == "" {
		return false
	}

	now := tm.nowFunc()
	// Token is invalid if it expires within the buffer period
	return tm.expiresAt.After(now.Add(TokenRefreshBuffer))
}
