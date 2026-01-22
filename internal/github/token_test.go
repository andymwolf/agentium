package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewTokenExchanger(t *testing.T) {
	exchanger := NewTokenExchanger()
	if exchanger == nil {
		t.Fatal("expected exchanger, got nil")
	}
	if exchanger.baseURL != "https://api.github.com" {
		t.Errorf("expected default baseURL, got %s", exchanger.baseURL)
	}
	if exchanger.httpClient == nil {
		t.Error("expected http client, got nil")
	}
}

func TestNewTokenExchanger_WithOptions(t *testing.T) {
	customClient := &http.Client{Timeout: 60 * time.Second}
	customURL := "https://custom.github.com"

	exchanger := NewTokenExchanger(
		WithHTTPClient(customClient),
		WithBaseURL(customURL),
	)

	if exchanger.httpClient != customClient {
		t.Error("expected custom http client")
	}
	if exchanger.baseURL != customURL {
		t.Errorf("expected baseURL %s, got %s", customURL, exchanger.baseURL)
	}
}

func TestExchangeToken_Success(t *testing.T) {
	expiresAt := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/app/installations/12345/access_tokens" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-jwt" {
			t.Errorf("unexpected auth header: %s", auth)
		}
		if accept := r.Header.Get("Accept"); accept != "application/vnd.github+json" {
			t.Errorf("unexpected accept header: %s", accept)
		}
		if version := r.Header.Get("X-GitHub-Api-Version"); version != "2022-11-28" {
			t.Errorf("unexpected API version header: %s", version)
		}

		// Return success response
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token":      "ghs_test_token_123",
			"expires_at": expiresAt.Format(time.RFC3339),
		})
	}))
	defer server.Close()

	exchanger := NewTokenExchanger(WithBaseURL(server.URL))
	token, err := exchanger.ExchangeToken("test-jwt", 12345)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == nil {
		t.Fatal("expected token, got nil")
	}
	if token.Token != "ghs_test_token_123" {
		t.Errorf("expected token ghs_test_token_123, got %s", token.Token)
	}
	if !token.ExpiresAt.Equal(expiresAt) {
		t.Errorf("expected expires_at %v, got %v", expiresAt, token.ExpiresAt)
	}
}

func TestExchangeToken_Validation(t *testing.T) {
	exchanger := NewTokenExchanger()

	tests := []struct {
		name           string
		jwt            string
		installationID int64
		wantErr        bool
		errContain     string
	}{
		{
			name:           "empty JWT",
			jwt:            "",
			installationID: 12345,
			wantErr:        true,
			errContain:     "JWT cannot be empty",
		},
		{
			name:           "zero installation ID",
			jwt:            "test-jwt",
			installationID: 0,
			wantErr:        true,
			errContain:     "installation ID must be positive",
		},
		{
			name:           "negative installation ID",
			jwt:            "test-jwt",
			installationID: -1,
			wantErr:        true,
			errContain:     "installation ID must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := exchanger.ExchangeToken(tt.jwt, tt.installationID)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("expected error containing %q, got %q", tt.errContain, err.Error())
				}
				if token != nil {
					t.Error("expected nil token on error")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestExchangeToken_APIErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   map[string]interface{}
		errContain string
	}{
		{
			name:       "unauthorized",
			statusCode: http.StatusUnauthorized,
			response: map[string]interface{}{
				"message":           "A JSON web token could not be decoded",
				"documentation_url": "https://docs.github.com",
			},
			errContain: "unauthorized",
		},
		{
			name:       "forbidden",
			statusCode: http.StatusForbidden,
			response: map[string]interface{}{
				"message":           "Resource not accessible by integration",
				"documentation_url": "https://docs.github.com",
			},
			errContain: "forbidden",
		},
		{
			name:       "not found",
			statusCode: http.StatusNotFound,
			response: map[string]interface{}{
				"message":           "Integration not found",
				"documentation_url": "https://docs.github.com",
			},
			errContain: "not found",
		},
		{
			name:       "server error",
			statusCode: http.StatusInternalServerError,
			response: map[string]interface{}{
				"message": "Internal server error",
			},
			errContain: "API error (status 500)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				json.NewEncoder(w).Encode(tt.response)
			}))
			defer server.Close()

			exchanger := NewTokenExchanger(WithBaseURL(server.URL))
			token, err := exchanger.ExchangeToken("test-jwt", 12345)

			if err == nil {
				t.Error("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errContain) {
				t.Errorf("expected error containing %q, got %q", tt.errContain, err.Error())
			}
			if token != nil {
				t.Error("expected nil token on error")
			}
		})
	}
}

func TestExchangeToken_InvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	exchanger := NewTokenExchanger(WithBaseURL(server.URL))
	token, err := exchanger.ExchangeToken("test-jwt", 12345)

	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse token response") {
		t.Errorf("unexpected error: %v", err)
	}
	if token != nil {
		t.Error("expected nil token on error")
	}
}

func TestExchangeToken_NetworkError(t *testing.T) {
	// Use an invalid URL to simulate network error
	exchanger := NewTokenExchanger(WithBaseURL("http://localhost:99999"))
	token, err := exchanger.ExchangeToken("test-jwt", 12345)

	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to make request") {
		t.Errorf("unexpected error: %v", err)
	}
	if token != nil {
		t.Error("expected nil token on error")
	}
}
