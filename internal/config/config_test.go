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

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}
