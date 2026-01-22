// Package github provides GitHub App authentication utilities.
package github

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// JWTGenerator generates JWT tokens for GitHub App authentication.
type JWTGenerator struct {
	appID      string
	privateKey *rsa.PrivateKey
}

// NewJWTGenerator creates a new JWT generator with the given App ID and private key PEM.
func NewJWTGenerator(appID string, privateKeyPEM []byte) (*JWTGenerator, error) {
	privateKey, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return &JWTGenerator{
		appID:      appID,
		privateKey: privateKey,
	}, nil
}

// GenerateToken creates a new JWT token valid for 10 minutes.
// The token can be used to authenticate as a GitHub App.
func (g *JWTGenerator) GenerateToken() (string, error) {
	return g.GenerateTokenWithDuration(10 * time.Minute)
}

// GenerateTokenWithDuration creates a new JWT token valid for the specified duration.
// GitHub allows JWTs to be valid for up to 10 minutes.
func (g *JWTGenerator) GenerateTokenWithDuration(duration time.Duration) (string, error) {
	now := time.Now()

	claims := jwt.RegisteredClaims{
		Issuer:    g.appID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(g.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return signedToken, nil
}

// parsePrivateKey parses a PEM-encoded RSA private key.
func parsePrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Try PKCS#1 format first (RSA PRIVATE KEY)
	if block.Type == "RSA PRIVATE KEY" {
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}

	// Try PKCS#8 format (PRIVATE KEY)
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}

	return rsaKey, nil
}
