package github

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// generateTestKeyPair generates an RSA key pair for testing.
func generateTestKeyPair(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	return privateKey, pemData
}

func TestNewJWTGenerator(t *testing.T) {
	_, pemData := generateTestKeyPair(t)

	tests := []struct {
		name       string
		appID      string
		pemData    []byte
		wantErr    bool
		errContain string
	}{
		{
			name:    "valid key",
			appID:   "12345",
			pemData: pemData,
			wantErr: false,
		},
		{
			name:       "empty app ID",
			appID:      "",
			pemData:    pemData,
			wantErr:    true,
			errContain: "app ID cannot be empty",
		},
		{
			name:       "invalid PEM data",
			appID:      "12345",
			pemData:    []byte("not a valid pem"),
			wantErr:    true,
			errContain: "failed to decode PEM block",
		},
		{
			name:       "empty PEM data",
			appID:      "12345",
			pemData:    []byte{},
			wantErr:    true,
			errContain: "failed to decode PEM block",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := NewJWTGenerator(tt.appID, tt.pemData)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("expected error containing %q, got %q", tt.errContain, err.Error())
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if gen == nil {
				t.Error("expected generator, got nil")
			}
		})
	}
}

func TestGenerateToken(t *testing.T) {
	privateKey, pemData := generateTestKeyPair(t)

	appID := "12345"
	gen, err := NewJWTGenerator(appID, pemData)
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	token, err := gen.GenerateToken()
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	if token == "" {
		t.Fatal("expected non-empty token")
	}

	// Parse and verify the token
	parsedToken, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		return &privateKey.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}

	if !parsedToken.Valid {
		t.Error("token is not valid")
	}

	// Verify signing method is RS256
	if parsedToken.Method.Alg() != "RS256" {
		t.Errorf("expected RS256, got %s", parsedToken.Method.Alg())
	}

	// Verify claims
	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("failed to get claims")
	}

	// Check issuer
	if iss, ok := claims["iss"].(string); !ok || iss != appID {
		t.Errorf("expected iss=%s, got %v", appID, claims["iss"])
	}

	// Check issued at
	if _, ok := claims["iat"]; !ok {
		t.Error("missing iat claim")
	}

	// Check expiration
	if _, ok := claims["exp"]; !ok {
		t.Error("missing exp claim")
	}
}

func TestGenerateTokenWithDuration(t *testing.T) {
	privateKey, pemData := generateTestKeyPair(t)

	gen, err := NewJWTGenerator("12345", pemData)
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	duration := 5 * time.Minute
	beforeGen := time.Now()

	token, err := gen.GenerateTokenWithDuration(duration)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	afterGen := time.Now()

	// Parse and verify the token
	parsedToken, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		return &privateKey.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("failed to get claims")
	}

	// Verify expiration is approximately duration from now
	// JWT timestamps are in seconds, so truncate for comparison
	expFloat, ok := claims["exp"].(float64)
	if !ok {
		t.Fatal("exp claim is not a number")
	}
	exp := time.Unix(int64(expFloat), 0)

	// Truncate to seconds since JWT uses second precision
	expectedExpMin := beforeGen.Truncate(time.Second).Add(duration)
	expectedExpMax := afterGen.Add(duration).Add(time.Second) // Add buffer for timing

	if exp.Before(expectedExpMin) || exp.After(expectedExpMax) {
		t.Errorf("exp %v not in expected range [%v, %v]", exp, expectedExpMin, expectedExpMax)
	}
}

func TestGenerateTokenWithDuration_Validation(t *testing.T) {
	_, pemData := generateTestKeyPair(t)

	gen, err := NewJWTGenerator("12345", pemData)
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	tests := []struct {
		name       string
		duration   time.Duration
		wantErr    bool
		errContain string
	}{
		{
			name:     "valid duration",
			duration: 5 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "max duration",
			duration: MaxJWTDuration,
			wantErr:  false,
		},
		{
			name:       "exceeds max duration",
			duration:   11 * time.Minute,
			wantErr:    true,
			errContain: "exceeds maximum allowed",
		},
		{
			name:       "zero duration",
			duration:   0,
			wantErr:    true,
			errContain: "duration must be positive",
		},
		{
			name:       "negative duration",
			duration:   -1 * time.Minute,
			wantErr:    true,
			errContain: "duration must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := gen.GenerateTokenWithDuration(tt.duration)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("expected error containing %q, got %q", tt.errContain, err.Error())
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if token == "" {
				t.Error("expected non-empty token")
			}
		})
	}
}

func TestParsePKCS8PrivateKey(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	// Encode as PKCS#8
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("failed to marshal PKCS8: %v", err)
	}

	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})

	gen, err := NewJWTGenerator("12345", pemData)
	if err != nil {
		t.Fatalf("failed to create generator with PKCS8 key: %v", err)
	}

	token, err := gen.GenerateToken()
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}
}
