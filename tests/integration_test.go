// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"nhooyr.io/websocket"

	"github.com/gogatehq/gogate/internal/config"
	"github.com/gogatehq/gogate/internal/gateway"
	"github.com/gogatehq/gogate/internal/ratelimit"
)

func TestGatewayRoutesByPrefixAndReturnsStructured404(t *testing.T) {
	t.Parallel()

	authBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("auth-service"))
	}))
	t.Cleanup(authBackend.Close)

	schoolBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("school-service"))
	}))
	t.Cleanup(schoolBackend.Close)

	handler := mustBuildHandler(t, &config.Config{
		Server: config.ServerConfig{Port: 8080},
		Services: []config.ServiceConfig{
			{Name: "auth", Prefix: "/api/v1/auth", Target: authBackend.URL, SkipAuth: boolPtr(true)},
			{Name: "school", Prefix: "/api/v1/schools", Target: schoolBackend.URL, SkipAuth: boolPtr(true)},
		},
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	authResp := mustDo(t, server.URL+"/api/v1/auth/login")
	if authResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from auth route, got %d", authResp.StatusCode)
	}
	if body := readBody(t, authResp.Body); body != "auth-service" {
		t.Fatalf("expected auth backend response, got %q", body)
	}

	schoolResp := mustDo(t, server.URL+"/api/v1/schools/list")
	if schoolResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from school route, got %d", schoolResp.StatusCode)
	}
	if body := readBody(t, schoolResp.Body); body != "school-service" {
		t.Fatalf("expected school backend response, got %q", body)
	}

	notFoundResp := mustDo(t, server.URL+"/unknown/path")
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown route, got %d", notFoundResp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(notFoundResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode 404 body: %v", err)
	}
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected success=false in 404 payload")
	}
	if errorText, _ := payload["error"].(string); !strings.Contains(errorText, "route not found") {
		t.Fatalf("expected route not found error, got %q", errorText)
	}
}

func TestGatewayInjectsRequestIDAndForwardsIt(t *testing.T) {
	t.Parallel()

	requestIDCh := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestIDCh <- r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	handler := mustBuildHandler(t, &config.Config{
		Server: config.ServerConfig{Port: 8080},
		Services: []config.ServiceConfig{
			{Name: "auth", Prefix: "/api/v1/auth", Target: backend.URL, SkipAuth: boolPtr(true)},
		},
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	resp := mustDo(t, server.URL+"/api/v1/auth/login")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	responseRequestID := resp.Header.Get("X-Request-ID")
	if responseRequestID == "" {
		t.Fatal("expected X-Request-ID response header")
	}

	forwardedRequestID := <-requestIDCh
	if forwardedRequestID == "" {
		t.Fatal("expected forwarded X-Request-ID to upstream")
	}
	if forwardedRequestID != responseRequestID {
		t.Fatalf("expected forwarded request ID to match response request ID, got %q vs %q", forwardedRequestID, responseRequestID)
	}
}

func TestGatewayProtectedRouteInjectsIdentityHeaders(t *testing.T) {
	t.Parallel()

	var userID string
	var tenantID string
	var roles string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID = r.Header.Get("X-User-ID")
		tenantID = r.Header.Get("X-Tenant-ID")
		roles = r.Header.Get("X-User-Roles")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	handler := mustBuildHandler(t, &config.Config{
		Server: config.ServerConfig{Port: 8080},
		JWT: config.JWTConfig{
			Issuer:     "api-gateway",
			Algorithms: []string{"HS256"},
			Keys: []config.JWTKeyConfig{
				{KID: "k1", KTY: "oct", Value: "secret-12345678901234567890123456789012", Primary: true},
			},
		},
		Services: []config.ServiceConfig{
			{Name: "auth", Prefix: "/api/v1/auth", Target: backend.URL, SkipAuth: boolPtr(false)},
		},
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	token := mustSignToken(t, "k1", "secret-12345678901234567890123456789012", "greenfield", time.Now().Add(1*time.Hour))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/v1/auth/me", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if userID != "user_1" {
		t.Fatalf("expected X-User-ID user_1, got %q", userID)
	}
	if tenantID != "greenfield" {
		t.Fatalf("expected X-Tenant-ID greenfield, got %q", tenantID)
	}
	if roles != "admin,teacher" {
		t.Fatalf("expected X-User-Roles admin,teacher, got %q", roles)
	}
}

func TestGatewayProtectedRouteRejectsExpiredToken(t *testing.T) {
	t.Parallel()

	var backendCalls atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		backendCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	handler := mustBuildHandler(t, &config.Config{
		Server: config.ServerConfig{Port: 8080},
		JWT: config.JWTConfig{
			Issuer:     "api-gateway",
			Algorithms: []string{"HS256"},
			Keys: []config.JWTKeyConfig{
				{KID: "k1", KTY: "oct", Value: "secret-12345678901234567890123456789012", Primary: true},
			},
		},
		Services: []config.ServiceConfig{
			{Name: "auth", Prefix: "/api/v1/auth", Target: backend.URL, SkipAuth: boolPtr(false)},
		},
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	token := mustSignToken(t, "k1", "secret-12345678901234567890123456789012", "greenfield", time.Now().Add(-1*time.Hour))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/v1/auth/me", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if backendCalls.Load() != 0 {
		t.Fatalf("expected backend not to be called, got %d calls", backendCalls.Load())
	}
}

func TestGatewayRejectsTenantMismatchOnTenantAwareRoute(t *testing.T) {
	t.Parallel()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	handler := mustBuildHandler(t, &config.Config{
		Server: config.ServerConfig{Port: 8080},
		JWT: config.JWTConfig{
			Issuer:     "api-gateway",
			Algorithms: []string{"HS256"},
			Keys: []config.JWTKeyConfig{
				{KID: "k1", KTY: "oct", Value: "secret-12345678901234567890123456789012", Primary: true},
			},
		},
		Tenant: config.TenantConfig{
			Strategy:           "subdomain",
			ReservedSubdomains: []string{"www", "api", "admin"},
		},
		Services: []config.ServiceConfig{
			{
				Name:        "school",
				Prefix:      "/api/v1/schools",
				Target:      backend.URL,
				SkipAuth:    boolPtr(false),
				TenantAware: boolPtr(true),
			},
		},
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	token := mustSignToken(t, "k1", "secret-12345678901234567890123456789012", "another-tenant", time.Now().Add(1*time.Hour))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/v1/schools/list", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = "greenfield.app.test"
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestGatewayStripsSpoofedHeadersAndInjectsCanonicalValues(t *testing.T) {
	t.Parallel()

	var forwardedUserID string
	var forwardedTenantID string
	var forwardedRoles string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwardedUserID = r.Header.Get("X-User-ID")
		forwardedTenantID = r.Header.Get("X-Tenant-ID")
		forwardedRoles = r.Header.Get("X-User-Roles")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	handler := mustBuildHandler(t, &config.Config{
		Server: config.ServerConfig{Port: 8080},
		JWT: config.JWTConfig{
			Issuer:     "api-gateway",
			Algorithms: []string{"HS256"},
			Keys: []config.JWTKeyConfig{
				{KID: "k1", KTY: "oct", Value: "secret-12345678901234567890123456789012", Primary: true},
			},
		},
		Tenant: config.TenantConfig{
			Strategy:           "subdomain",
			ReservedSubdomains: []string{"www", "api", "admin"},
		},
		Services: []config.ServiceConfig{
			{
				Name:        "school",
				Prefix:      "/api/v1/schools",
				Target:      backend.URL,
				SkipAuth:    boolPtr(false),
				TenantAware: boolPtr(true),
			},
		},
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	token := mustSignToken(t, "k1", "secret-12345678901234567890123456789012", "greenfield", time.Now().Add(1*time.Hour))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/v1/schools/list", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = "greenfield.app.test"
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-User-ID", "spoofed-user")
	req.Header.Set("X-Tenant-ID", "spoofed-tenant")
	req.Header.Set("X-User-Roles", "superadmin")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if forwardedUserID != "user_1" {
		t.Fatalf("expected canonical user ID, got %q", forwardedUserID)
	}
	if forwardedTenantID != "greenfield" {
		t.Fatalf("expected canonical tenant ID, got %q", forwardedTenantID)
	}
	if forwardedRoles != "admin,teacher" {
		t.Fatalf("expected canonical roles, got %q", forwardedRoles)
	}
}

func TestGatewayRateLimitsAndReturns429(t *testing.T) {
	t.Parallel()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	mr := miniredis.RunT(t)
	rpm := 3
	limiter, err := ratelimit.NewLimiter(
		config.RedisConfig{Addr: mr.Addr(), DialTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second},
		config.RateLimitConfig{DefaultRPM: rpm, KeyPrefix: "test:rl:"},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	t.Cleanup(func() { limiter.Close() })

	cfg := &config.Config{
		Server:    config.ServerConfig{Port: 8080},
		RateLimit: config.RateLimitConfig{DefaultRPM: rpm, KeyPrefix: "test:rl:"},
		Services: []config.ServiceConfig{
			{Name: "api", Prefix: "/api", Target: backend.URL, SkipAuth: boolPtr(true)},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, err := gateway.NewHandler(cfg, logger, &gateway.HandlerDeps{Limiter: limiter})
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	// First 3 requests should succeed.
	for i := 0; i < rpm; i++ {
		resp := mustDo(t, server.URL+"/api/test")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, resp.StatusCode)
		}
		if resp.Header.Get("X-RateLimit-Limit") == "" {
			t.Fatalf("request %d: missing X-RateLimit-Limit header", i+1)
		}
	}

	// 4th request should be rate limited.
	resp := mustDo(t, server.URL+"/api/test")
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}
	if resp.Header.Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("expected X-RateLimit-Remaining 0, got %q", resp.Header.Get("X-RateLimit-Remaining"))
	}
}

func TestGatewaySecurityHeadersPresent(t *testing.T) {
	t.Parallel()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	handler := mustBuildHandler(t, &config.Config{
		Server: config.ServerConfig{Port: 8080},
		Services: []config.ServiceConfig{
			{Name: "api", Prefix: "/api", Target: backend.URL, SkipAuth: boolPtr(true)},
		},
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	resp := mustDo(t, server.URL+"/api/test")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	expectations := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "0",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for header, expected := range expectations {
		if got := resp.Header.Get(header); got != expected {
			t.Errorf("expected %s=%q, got %q", header, expected, got)
		}
	}

	// Security headers should also be present on system endpoints.
	healthResp := mustDo(t, server.URL+"/health")
	if healthResp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected security headers on /health")
	}
}

func TestGatewayWebSocketUpgradeProxy(t *testing.T) {
	t.Parallel()

	// Backend that accepts WebSocket upgrades and echoes messages back.
	wsBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		ctx := r.Context()
		for {
			msgType, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if err := conn.Write(ctx, msgType, data); err != nil {
				return
			}
		}
	}))
	t.Cleanup(wsBackend.Close)

	handler := mustBuildHandler(t, &config.Config{
		Server: config.ServerConfig{Port: 8080},
		Services: []config.ServiceConfig{
			{Name: "ws", Prefix: "/ws", Target: wsBackend.URL, SkipAuth: boolPtr(true)},
		},
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	// Connect via WebSocket through the gateway.
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/echo"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial through gateway: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	// Send a message and verify it echoes back.
	testMsg := "hello from gogate"
	if err := conn.Write(ctx, websocket.MessageText, []byte(testMsg)); err != nil {
		t.Fatalf("websocket write: %v", err)
	}

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("websocket read: %v", err)
	}
	if string(data) != testMsg {
		t.Fatalf("expected echo %q, got %q", testMsg, string(data))
	}
}

func TestGatewayStripPrefixBeforeForwarding(t *testing.T) {
	t.Parallel()

	var receivedPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	handler := mustBuildHandler(t, &config.Config{
		Server: config.ServerConfig{Port: 8080},
		Services: []config.ServiceConfig{
			{Name: "users", Prefix: "/api/v1/users", Target: backend.URL, SkipAuth: boolPtr(true), StripPrefix: boolPtr(true)},
		},
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	resp := mustDo(t, server.URL+"/api/v1/users/123/profile")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if receivedPath != "/123/profile" {
		t.Fatalf("expected stripped path /123/profile, got %q", receivedPath)
	}
}

func TestGatewayPreservesPrefixByDefault(t *testing.T) {
	t.Parallel()

	var receivedPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	handler := mustBuildHandler(t, &config.Config{
		Server: config.ServerConfig{Port: 8080},
		Services: []config.ServiceConfig{
			{Name: "users", Prefix: "/api/v1/users", Target: backend.URL, SkipAuth: boolPtr(true)},
		},
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	resp := mustDo(t, server.URL+"/api/v1/users/123/profile")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if receivedPath != "/api/v1/users/123/profile" {
		t.Fatalf("expected full path preserved, got %q", receivedPath)
	}
}

func TestGatewayStripsAuthorizationHeaderBeforeForwarding(t *testing.T) {
	t.Parallel()

	var forwardedAuth string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwardedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	handler := mustBuildHandler(t, &config.Config{
		Server: config.ServerConfig{Port: 8080},
		JWT: config.JWTConfig{
			Issuer:     "api-gateway",
			Algorithms: []string{"HS256"},
			Keys: []config.JWTKeyConfig{
				{KID: "k1", KTY: "oct", Value: "secret-12345678901234567890123456789012", Primary: true},
			},
		},
		Services: []config.ServiceConfig{
			{Name: "api", Prefix: "/api", Target: backend.URL, SkipAuth: boolPtr(false)},
		},
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	token := mustSignToken(t, "k1", "secret-12345678901234567890123456789012", "greenfield", time.Now().Add(1*time.Hour))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/test", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if forwardedAuth != "" {
		t.Fatalf("expected Authorization header to be stripped, got %q", forwardedAuth)
	}
}

func mustBuildHandler(t *testing.T, cfg *config.Config) http.Handler {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, err := gateway.NewHandler(cfg, logger, nil)
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	return handler
}

func mustDo(t *testing.T, rawURL string) *http.Response {
	t.Helper()

	resp, err := http.Get(rawURL)
	if err != nil {
		t.Fatalf("http get %s: %v", rawURL, err)
	}
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})
	return resp
}

func readBody(t *testing.T, body io.ReadCloser) string {
	t.Helper()

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func boolPtr(value bool) *bool {
	return &value
}

func mustSignToken(t *testing.T, kid string, secret string, tenantID string, expiresAt time.Time) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":   "user_1",
		"tenant_id": tenantID,
		"roles":     []string{"admin", "teacher"},
		"iss":       "api-gateway",
		"iat":       time.Now().Unix(),
		"exp":       expiresAt.Unix(),
	})
	token.Header["kid"] = kid

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tokenString
}
