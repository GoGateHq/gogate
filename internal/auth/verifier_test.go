// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/gogatehq/gogate/internal/config"
)

func TestVerifyValidToken(t *testing.T) {
	t.Parallel()

	verifier := NewVerifier(config.JWTConfig{
		Issuer:     "api-gateway",
		Algorithms: []string{"HS256"},
		Keys: []config.JWTKeyConfig{
			{KID: "k1", KTY: "oct", Value: "secret-12345678901234567890123456789012", Primary: true},
		},
	})

	token := mustSignHS256Token(t, "k1", "secret-12345678901234567890123456789012", time.Now().Add(1*time.Hour))
	identity, err := verifier.Verify(context.Background(), "Bearer "+token)
	if err != nil {
		t.Fatalf("expected token to validate, got error: %v", err)
	}
	if identity.UserID != "user_1" {
		t.Fatalf("expected user_1, got %q", identity.UserID)
	}
	if identity.TenantID != "greenfield" {
		t.Fatalf("expected greenfield, got %q", identity.TenantID)
	}
}

func TestVerifyExpiredToken(t *testing.T) {
	t.Parallel()

	verifier := NewVerifier(config.JWTConfig{
		Issuer:     "api-gateway",
		Algorithms: []string{"HS256"},
		Keys: []config.JWTKeyConfig{
			{KID: "k1", KTY: "oct", Value: "secret-12345678901234567890123456789012", Primary: true},
		},
	})

	token := mustSignHS256Token(t, "k1", "secret-12345678901234567890123456789012", time.Now().Add(-1*time.Hour))
	if _, err := verifier.Verify(context.Background(), "Bearer "+token); err == nil {
		t.Fatal("expected expired token to fail")
	}
}

func TestVerifyMissingHeader(t *testing.T) {
	t.Parallel()

	verifier := NewVerifier(config.JWTConfig{
		Issuer:     "api-gateway",
		Algorithms: []string{"HS256"},
		Keys: []config.JWTKeyConfig{
			{KID: "k1", KTY: "oct", Value: "secret-12345678901234567890123456789012", Primary: true},
		},
	})

	if _, err := verifier.Verify(context.Background(), ""); err == nil {
		t.Fatal("expected missing header to fail")
	}
}

func TestVerifyWrongSecret(t *testing.T) {
	t.Parallel()

	verifier := NewVerifier(config.JWTConfig{
		Issuer:     "api-gateway",
		Algorithms: []string{"HS256"},
		Keys: []config.JWTKeyConfig{
			{KID: "k1", KTY: "oct", Value: "secret-12345678901234567890123456789012", Primary: true},
		},
	})

	token := mustSignHS256Token(t, "k1", "another-secret-1234567890123456789012", time.Now().Add(1*time.Hour))
	if _, err := verifier.Verify(context.Background(), "Bearer "+token); err == nil {
		t.Fatal("expected wrong secret token to fail")
	}
}

func TestVerifyRejectsNoneAlgorithm(t *testing.T) {
	t.Parallel()

	verifier := NewVerifier(config.JWTConfig{
		Issuer:     "api-gateway",
		Algorithms: []string{"HS256"},
		Keys: []config.JWTKeyConfig{
			{KID: "k1", KTY: "oct", Value: "secret-12345678901234567890123456789012", Primary: true},
		},
	})

	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"user_id":   "user_1",
		"tenant_id": "greenfield",
		"roles":     []string{"admin"},
		"iss":       "api-gateway",
		"exp":       time.Now().Add(1 * time.Hour).Unix(),
	})
	token.Header["kid"] = "k1"

	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none token: %v", err)
	}

	if _, err := verifier.Verify(context.Background(), "Bearer "+tokenString); err == nil {
		t.Fatal("expected none algorithm token to fail")
	}
}

func TestVerifySupportsRotationWithMultipleKeys(t *testing.T) {
	t.Parallel()

	verifier := NewVerifier(config.JWTConfig{
		Issuer:     "api-gateway",
		Algorithms: []string{"HS256"},
		Keys: []config.JWTKeyConfig{
			{KID: "old", KTY: "oct", Value: "old-secret-1234567890123456789012345678"},
			{KID: "new", KTY: "oct", Value: "new-secret-1234567890123456789012345678", Primary: true},
		},
	})

	oldToken := mustSignHS256Token(t, "old", "old-secret-1234567890123456789012345678", time.Now().Add(1*time.Hour))
	if _, err := verifier.Verify(context.Background(), "Bearer "+oldToken); err != nil {
		t.Fatalf("expected old key token to validate, got: %v", err)
	}

	newToken := mustSignHS256Token(t, "new", "new-secret-1234567890123456789012345678", time.Now().Add(1*time.Hour))
	if _, err := verifier.Verify(context.Background(), "Bearer "+newToken); err != nil {
		t.Fatalf("expected new key token to validate, got: %v", err)
	}
}

func TestVerifyFetchesJWKSWhenKidNotInLocalStore(t *testing.T) {
	t.Parallel()

	remoteSecret := []byte("remote-secret-1234567890123456789012345678")
	remoteSecretB64 := base64.RawURLEncoding.EncodeToString(remoteSecret)

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{
				{"kid": "remote-kid", "kty": "oct", "k": remoteSecretB64},
			},
		})
	}))
	t.Cleanup(jwksServer.Close)

	verifier := NewVerifier(config.JWTConfig{
		Issuer:       "api-gateway",
		Algorithms:   []string{"HS256"},
		JWKSURL:      jwksServer.URL,
		JWKSCacheTTL: time.Minute,
	})

	token := mustSignHS256Token(t, "remote-kid", string(remoteSecret), time.Now().Add(1*time.Hour))
	if _, err := verifier.Verify(context.Background(), "Bearer "+token); err != nil {
		t.Fatalf("expected JWKS token to validate, got: %v", err)
	}
}

func mustSignHS256Token(t *testing.T, kid string, secret string, expiration time.Time) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":   "user_1",
		"tenant_id": "greenfield",
		"roles":     []string{"admin"},
		"iss":       "api-gateway",
		"exp":       expiration.Unix(),
		"iat":       time.Now().Unix(),
	})
	token.Header["kid"] = kid

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tokenString
}
