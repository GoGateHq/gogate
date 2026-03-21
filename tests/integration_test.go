// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package tests

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gogatehq/gogate/internal/config"
	"github.com/gogatehq/gogate/internal/gateway"
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
			{Name: "auth", Prefix: "/api/v1/auth", Target: authBackend.URL},
			{Name: "school", Prefix: "/api/v1/schools", Target: schoolBackend.URL},
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
			{Name: "auth", Prefix: "/api/v1/auth", Target: backend.URL},
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

func mustBuildHandler(t *testing.T, cfg *config.Config) http.Handler {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, err := gateway.NewHandler(cfg, logger)
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
