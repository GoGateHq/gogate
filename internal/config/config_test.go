// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
server:
  port: 8080
services:
  - name: auth
    prefix: /api/v1/auth
    target: http://localhost:8081
    skip_auth: true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Fatalf("expected port 8080, got %d", cfg.Server.Port)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(cfg.Services))
	}
	if !cfg.Services[0].IsAuthSkipped() {
		t.Fatal("expected skip_auth to be parsed as true")
	}
}

func TestLoadFailsOnInvalidPort(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
server:
  port: 0
`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "server.port") {
		t.Fatalf("expected server.port validation error, got: %v", err)
	}
}

func TestLoadFailsOnInvalidServiceTarget(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
server:
  port: 8080
services:
  - name: auth
    prefix: /api/v1/auth
    target: ftp://localhost:8081
`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "http or https") {
		t.Fatalf("expected target scheme validation error, got: %v", err)
	}
}

func TestLoadFailsOnDuplicatePrefixes(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
server:
  port: 8080
services:
  - name: auth
    prefix: /api/v1/auth
    target: http://localhost:8081
  - name: auth2
    prefix: /api/v1/auth
    target: http://localhost:8082
`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("expected duplicate prefix validation error, got: %v", err)
	}
}

func TestLoadFailsOnUnknownFields(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
server:
  port: 8080
  does_not_exist: true
`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "field does_not_exist not found") {
		t.Fatalf("expected unknown field error, got: %v", err)
	}
}

func TestLoadFailsOnInvalidTrustedProxyCIDR(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
server:
  port: 8080
  trusted_proxies:
    - invalid-cidr
`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "trusted_proxies") {
		t.Fatalf("expected trusted_proxies validation error, got: %v", err)
	}
}

func TestLoadFailsOnInvalidTenantStrategy(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
server:
  port: 8080
tenant:
  strategy: unknown
`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "tenant.strategy") {
		t.Fatalf("expected tenant.strategy validation error, got: %v", err)
	}
}

func TestLoadFailsOnUnsupportedJWTAlgorithm(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
server:
  port: 8080
jwt:
  algorithms:
    - HS512
`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "jwt.algorithms") {
		t.Fatalf("expected jwt.algorithms validation error, got: %v", err)
	}
}

func TestLoadFailsWhenHeaderStrategyHasNoHeaderName(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
server:
  port: 8080
tenant:
  strategy: header
`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "tenant.header_name") {
		t.Fatalf("expected tenant.header_name validation error, got: %v", err)
	}
}

func TestLoadDefaultsJWTKeyTypeToOct(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
server:
  port: 8080
jwt:
  keys:
    - kid: k1
      value: abc12345678901234567890123456789
      primary: true
services:
  - name: auth
    prefix: /api/v1/auth
    target: http://localhost:8081
    skip_auth: false
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
	if cfg.JWT.Keys[0].KTY != "oct" {
		t.Fatalf("expected KTY to default to oct, got %q", cfg.JWT.Keys[0].KTY)
	}
}

func TestLoadExpandsEnvironmentVariables(t *testing.T) {
	t.Setenv("JWT_SIGNING_KEY", "env-secret-1234567890123456789012345678")
	path := writeTempConfig(t, `
server:
  port: 8080
jwt:
  keys:
    - kid: k1
      value: ${JWT_SIGNING_KEY}
      primary: true
services:
  - name: auth
    prefix: /api/v1/auth
    target: http://localhost:8081
    skip_auth: false
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected env-expanded config to load, got: %v", err)
	}
	if got := cfg.JWT.Keys[0].Value; got != "env-secret-1234567890123456789012345678" {
		t.Fatalf("expected expanded env value, got %q", got)
	}
}

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}
