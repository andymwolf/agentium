package github

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// generateTestKeyPairForManager generates an RSA key pair for testing.
func generateTestKeyPairForManager(t *testing.T) []byte {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	return pemData
}

func TestNewTokenManager(t *testing.T) {
	pemData := generateTestKeyPairForManager(t)

	tests := []struct {
		name           string
		appID          string
		installationID int64
		privateKey     []byte
		wantErr        bool
		errContain     string
	}{
		{
			name:           "valid parameters",
			appID:          "12345",
			installationID: 67890,
			privateKey:     pemData,
			wantErr:        false,
		},
		{
			name:           "empty app ID",
			appID:          "",
			installationID: 67890,
			privateKey:     pemData,
			wantErr:        true,
			errContain:     "app ID cannot be empty",
		},
		{
			name:           "zero installation ID",
			appID:          "12345",
			installationID: 0,
			privateKey:     pemData,
			wantErr:        true,
			errContain:     "installation ID must be positive",
		},
		{
			name:           "negative installation ID",
			appID:          "12345",
			installationID: -1,
			privateKey:     pemData,
			wantErr:        true,
			errContain:     "installation ID must be positive",
		},
		{
			name:           "empty private key",
			appID:          "12345",
			installationID: 67890,
			privateKey:     []byte{},
			wantErr:        true,
			errContain:     "private key cannot be empty",
		},
		{
			name:           "nil private key",
			appID:          "12345",
			installationID: 67890,
			privateKey:     nil,
			wantErr:        true,
			errContain:     "private key cannot be empty",
		},
		{
			name:           "invalid private key",
			appID:          "12345",
			installationID: 67890,
			privateKey:     []byte("not a valid pem"),
			wantErr:        true,
			errContain:     "failed to create JWT generator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm, err := NewTokenManager(tt.appID, tt.installationID, tt.privateKey)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("expected error containing %q, got %q", tt.errContain, err.Error())
				}
				if tm != nil {
					t.Error("expected nil TokenManager on error")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tm == nil {
				t.Error("expected TokenManager, got nil")
			}
		})
	}
}

func TestTokenManager_Token(t *testing.T) {
	pemData := generateTestKeyPairForManager(t)
	expiresAt := time.Now().Add(1 * time.Hour).UTC()

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token":      "ghs_test_token_123",
			"expires_at": expiresAt.Format(time.RFC3339),
		})
	}))
	defer server.Close()

	// Create TokenManager with custom exchanger
	exchanger := NewTokenExchanger(WithBaseURL(server.URL))
	tm, err := NewTokenManager("12345", 67890, pemData, WithTokenExchanger(exchanger))
	if err != nil {
		t.Fatalf("failed to create TokenManager: %v", err)
	}

	// First call should fetch a new token
	token, err := tm.Token()
	if err != nil {
		t.Fatalf("failed to get token: %v", err)
	}
	if token != "ghs_test_token_123" {
		t.Errorf("expected token ghs_test_token_123, got %s", token)
	}

	// Second call should return cached token
	token2, err := tm.Token()
	if err != nil {
		t.Fatalf("failed to get cached token: %v", err)
	}
	if token2 != token {
		t.Errorf("expected cached token %s, got %s", token, token2)
	}
}

func TestTokenManager_NeedsRefresh(t *testing.T) {
	pemData := generateTestKeyPairForManager(t)

	tests := []struct {
		name         string
		tokenState   string // "none", "valid", "expiring_soon", "expired"
		wantNeedsRef bool
	}{
		{
			name:         "no token",
			tokenState:   "none",
			wantNeedsRef: true,
		},
		{
			name:         "valid token",
			tokenState:   "valid",
			wantNeedsRef: false,
		},
		{
			name:         "token expiring soon",
			tokenState:   "expiring_soon",
			wantNeedsRef: true,
		},
		{
			name:         "expired token",
			tokenState:   "expired",
			wantNeedsRef: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock server that returns tokens
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var expiresAt time.Time
				switch tt.tokenState {
				case "valid":
					expiresAt = time.Now().Add(1 * time.Hour)
				case "expiring_soon":
					expiresAt = time.Now().Add(2 * time.Minute) // Within 5-minute buffer
				case "expired":
					expiresAt = time.Now().Add(-1 * time.Minute)
				default:
					// For "none", we don't call the server
				}

				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"token":      "ghs_test_token",
					"expires_at": expiresAt.Format(time.RFC3339),
				})
			}))
			defer server.Close()

			exchanger := NewTokenExchanger(WithBaseURL(server.URL))
			tm, err := NewTokenManager("12345", 67890, pemData, WithTokenExchanger(exchanger))
			if err != nil {
				t.Fatalf("failed to create TokenManager: %v", err)
			}

			// Fetch initial token if not testing "none" state
			if tt.tokenState != "none" {
				_, _ = tm.Token()
			}

			got := tm.NeedsRefresh()
			if got != tt.wantNeedsRef {
				t.Errorf("NeedsRefresh() = %v, want %v", got, tt.wantNeedsRef)
			}
		})
	}
}

func TestTokenManager_Refresh(t *testing.T) {
	pemData := generateTestKeyPairForManager(t)
	callCount := 0

	// Create mock server that returns different tokens on each call
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token":      "ghs_token_" + string(rune('A'+callCount-1)),
			"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		})
	}))
	defer server.Close()

	exchanger := NewTokenExchanger(WithBaseURL(server.URL))
	tm, err := NewTokenManager("12345", 67890, pemData, WithTokenExchanger(exchanger))
	if err != nil {
		t.Fatalf("failed to create TokenManager: %v", err)
	}

	// Initial token
	token1, err := tm.Token()
	if err != nil {
		t.Fatalf("failed to get initial token: %v", err)
	}
	if token1 != "ghs_token_A" {
		t.Errorf("expected ghs_token_A, got %s", token1)
	}

	// Force refresh
	token2, err := tm.Refresh()
	if err != nil {
		t.Fatalf("failed to refresh token: %v", err)
	}
	if token2 != "ghs_token_B" {
		t.Errorf("expected ghs_token_B, got %s", token2)
	}
	if token2 == token1 {
		t.Error("expected different token after refresh")
	}

	// Verify the refreshed token is now cached
	token3, err := tm.Token()
	if err != nil {
		t.Fatalf("failed to get cached token: %v", err)
	}
	if token3 != token2 {
		t.Errorf("expected cached token %s, got %s", token2, token3)
	}
}

func TestTokenManager_ExpiresAt(t *testing.T) {
	pemData := generateTestKeyPairForManager(t)
	expiresAt := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token":      "ghs_test_token",
			"expires_at": expiresAt.Format(time.RFC3339),
		})
	}))
	defer server.Close()

	exchanger := NewTokenExchanger(WithBaseURL(server.URL))
	tm, err := NewTokenManager("12345", 67890, pemData, WithTokenExchanger(exchanger))
	if err != nil {
		t.Fatalf("failed to create TokenManager: %v", err)
	}

	// Before fetching, ExpiresAt should be zero
	if !tm.ExpiresAt().IsZero() {
		t.Error("expected zero ExpiresAt before fetching token")
	}

	// Fetch token
	_, _ = tm.Token()

	// After fetching, ExpiresAt should match
	gotExpiresAt := tm.ExpiresAt()
	if !gotExpiresAt.Equal(expiresAt) {
		t.Errorf("expected ExpiresAt %v, got %v", expiresAt, gotExpiresAt)
	}
}

func TestTokenManager_WithNowFunc(t *testing.T) {
	pemData := generateTestKeyPairForManager(t)

	// Simulate current time and a future time
	currentTime := time.Now()
	futureTime := currentTime.Add(1 * time.Hour)

	// Token expires in 30 minutes from "current" time
	expiresAt := currentTime.Add(30 * time.Minute)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token":      "ghs_test_token",
			"expires_at": expiresAt.Format(time.RFC3339),
		})
	}))
	defer server.Close()

	// Create TokenManager with mocked time
	mockNow := currentTime
	exchanger := NewTokenExchanger(WithBaseURL(server.URL))
	tm, err := NewTokenManager("12345", 67890, pemData,
		WithTokenExchanger(exchanger),
		WithNowFunc(func() time.Time { return mockNow }),
	)
	if err != nil {
		t.Fatalf("failed to create TokenManager: %v", err)
	}

	// Fetch initial token
	_, _ = tm.Token()

	// At current time, token is valid (30 min left, well beyond 5 min buffer)
	if tm.NeedsRefresh() {
		t.Error("token should be valid at current time")
	}

	// Advance time to 24 minutes later (6 minutes left, just outside 5-min buffer)
	mockNow = currentTime.Add(24 * time.Minute)
	if tm.NeedsRefresh() {
		t.Error("token should still be valid with 6 minutes left (outside buffer)")
	}

	// Advance time to 25 minutes later (5 minutes left, exactly at buffer boundary)
	// At the boundary, token expires at now + buffer, so !After means needs refresh
	mockNow = currentTime.Add(25 * time.Minute)
	if !tm.NeedsRefresh() {
		t.Error("token should need refresh at exactly 5 minutes left (boundary)")
	}

	// Advance time to 26 minutes later (4 min left, inside buffer)
	mockNow = currentTime.Add(26 * time.Minute)
	if !tm.NeedsRefresh() {
		t.Error("token should need refresh with only 4 minutes left")
	}

	// At future time (past expiry), token needs refresh
	mockNow = futureTime
	if !tm.NeedsRefresh() {
		t.Error("token should need refresh after expiry")
	}
}

func TestTokenManager_RefreshError(t *testing.T) {
	pemData := generateTestKeyPairForManager(t)

	// Create mock server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Bad credentials",
		})
	}))
	defer server.Close()

	exchanger := NewTokenExchanger(WithBaseURL(server.URL))
	tm, err := NewTokenManager("12345", 67890, pemData, WithTokenExchanger(exchanger))
	if err != nil {
		t.Fatalf("failed to create TokenManager: %v", err)
	}

	// Token() should return error
	_, err = tm.Token()
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to exchange token") {
		t.Errorf("expected exchange token error, got: %v", err)
	}

	// Refresh() should also return error
	_, err = tm.Refresh()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestTokenRefreshBuffer(t *testing.T) {
	// Verify the constant value
	if TokenRefreshBuffer != 5*time.Minute {
		t.Errorf("expected TokenRefreshBuffer to be 5 minutes, got %v", TokenRefreshBuffer)
	}
}
