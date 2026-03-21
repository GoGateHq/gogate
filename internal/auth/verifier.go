// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/gogatehq/gogate/internal/config"
)

var (
	ErrMissingBearerToken = errors.New("missing bearer token")
	ErrInvalidToken       = errors.New("invalid token")
)

const maxJWKSDocSize = 1 << 20 // 1 MiB

type VerifiedIdentity struct {
	UserID   string
	TenantID string
	Roles    []string
}

type Verifier struct {
	issuer         string
	allowedMethods []string
	clockSkew      time.Duration
	keys           *keyStore
}

type keyStore struct {
	local      map[string][]byte
	primaryKid string
	jwksURL    string
	cacheTTL   time.Duration
	client     *http.Client

	mu            sync.RWMutex
	cachedJWKS    map[string][]byte
	jwksExpiresAt time.Time
}

func NewVerifier(cfg config.JWTConfig) *Verifier {
	return &Verifier{
		issuer:         cfg.Issuer,
		allowedMethods: cfg.Algorithms,
		clockSkew:      cfg.ClockSkew,
		keys:           newKeyStore(cfg),
	}
}

func (v *Verifier) Verify(ctx context.Context, authorizationHeader string) (*VerifiedIdentity, error) {
	tokenString, err := bearerToken(authorizationHeader)
	if err != nil {
		return nil, err
	}

	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			kid, _ := token.Header["kid"].(string)
			key, err := v.keys.resolve(ctx, kid)
			if err != nil {
				return nil, err
			}
			return key, nil
		},
		jwt.WithValidMethods(v.allowedMethods),
		jwt.WithIssuer(v.issuer),
		jwt.WithLeeway(v.clockSkew),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	userID, _ := claims["user_id"].(string)
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("%w: missing user_id claim", ErrInvalidToken)
	}
	tenantID, _ := claims["tenant_id"].(string)

	return &VerifiedIdentity{
		UserID:   userID,
		TenantID: tenantID,
		Roles:    rolesFromClaim(claims["roles"]),
	}, nil
}

func bearerToken(authorizationHeader string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(authorizationHeader), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", ErrMissingBearerToken
	}
	return strings.TrimSpace(parts[1]), nil
}

func rolesFromClaim(raw any) []string {
	values, ok := raw.([]any)
	if !ok {
		return nil
	}

	roles := make([]string, 0, len(values))
	for _, value := range values {
		role, ok := value.(string)
		if !ok || strings.TrimSpace(role) == "" {
			continue
		}
		roles = append(roles, role)
	}
	return roles
}

func newKeyStore(cfg config.JWTConfig) *keyStore {
	local := make(map[string][]byte, len(cfg.Keys))
	primaryKid := ""
	for _, key := range cfg.Keys {
		local[key.KID] = []byte(key.Value)
		if key.Primary {
			primaryKid = key.KID
		}
	}

	return &keyStore{
		local:      local,
		primaryKid: primaryKid,
		jwksURL:    cfg.JWKSURL,
		cacheTTL:   cfg.JWKSCacheTTL,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (k *keyStore) resolve(ctx context.Context, kid string) ([]byte, error) {
	if kid != "" {
		if key, ok := k.local[kid]; ok {
			return key, nil
		}
		if key, ok := k.lookupJWKS(ctx, kid); ok {
			return key, nil
		}
		return nil, fmt.Errorf("no key for kid %q", kid)
	}

	if k.primaryKid != "" {
		if key, ok := k.local[k.primaryKid]; ok {
			return key, nil
		}
	}
	if len(k.local) == 1 {
		for _, key := range k.local {
			return key, nil
		}
	}

	// Attempt cache refresh when JWKS is configured but token has no kid.
	if k.jwksURL != "" {
		if keys := k.allJWKS(ctx); len(keys) == 1 {
			for _, key := range keys {
				return key, nil
			}
		}
	}

	return nil, errors.New("unable to resolve signing key")
}

func (k *keyStore) lookupJWKS(ctx context.Context, kid string) ([]byte, bool) {
	keys := k.allJWKS(ctx)
	key, ok := keys[kid]
	return key, ok
}

func (k *keyStore) allJWKS(ctx context.Context) map[string][]byte {
	if k.jwksURL == "" {
		return nil
	}

	k.mu.RLock()
	if time.Now().Before(k.jwksExpiresAt) && len(k.cachedJWKS) > 0 {
		defer k.mu.RUnlock()
		return copyKeyMap(k.cachedJWKS)
	}
	k.mu.RUnlock()

	keys, err := k.fetchJWKS(ctx)
	if err != nil {
		k.mu.RLock()
		defer k.mu.RUnlock()
		return copyKeyMap(k.cachedJWKS)
	}

	k.mu.Lock()
	k.cachedJWKS = keys
	k.jwksExpiresAt = time.Now().Add(k.cacheTTL)
	k.mu.Unlock()
	return copyKeyMap(keys)
}

func copyKeyMap(source map[string][]byte) map[string][]byte {
	if len(source) == 0 {
		return nil
	}

	copyMap := make(map[string][]byte, len(source))
	for key, value := range source {
		copyMap[key] = value
	}
	return copyMap
}

type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	KTY string `json:"kty"`
	KID string `json:"kid"`
	K   string `json:"k"`
}

func (k *keyStore) fetchJWKS(ctx context.Context) (map[string][]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.jwksURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := k.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks endpoint returned status %d", resp.StatusCode)
	}

	var document jwksDocument
	limitedReader := io.LimitReader(resp.Body, maxJWKSDocSize)
	if err := json.NewDecoder(limitedReader).Decode(&document); err != nil {
		return nil, err
	}

	keys := make(map[string][]byte)
	for _, item := range document.Keys {
		if item.KTY != "oct" || strings.TrimSpace(item.KID) == "" || strings.TrimSpace(item.K) == "" {
			continue
		}
		decoded, err := base64.RawURLEncoding.DecodeString(item.K)
		if err != nil {
			continue
		}
		keys[item.KID] = decoded
	}

	return keys, nil
}
