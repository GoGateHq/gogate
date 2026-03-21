// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package tenant

import (
	"net/http/httptest"
	"testing"

	"github.com/gogatehq/gogate/internal/config"
)

func TestResolveSubdomainStrategy(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(config.TenantConfig{
		Strategy:           "subdomain",
		ReservedSubdomains: []string{"www", "api", "admin"},
	})

	req := httptest.NewRequest("GET", "http://greenfield.app.test/path", nil)
	tenantID, err := resolver.Resolve(req)
	if err != nil {
		t.Fatalf("expected tenant to resolve, got error: %v", err)
	}
	if tenantID != "greenfield" {
		t.Fatalf("expected greenfield, got %q", tenantID)
	}
}

func TestResolveSubdomainStrategyRejectsReserved(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(config.TenantConfig{
		Strategy:           "subdomain",
		ReservedSubdomains: []string{"www", "api", "admin"},
	})

	req := httptest.NewRequest("GET", "http://www.app.test/path", nil)
	if _, err := resolver.Resolve(req); err == nil {
		t.Fatal("expected reserved subdomain to fail")
	}
}

func TestResolveHeaderStrategy(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(config.TenantConfig{
		Strategy:   "header",
		HeaderName: "X-Tenant-ID",
	})

	req := httptest.NewRequest("GET", "http://app.test/path", nil)
	req.Header.Set("X-Tenant-ID", "greenfield")
	tenantID, err := resolver.Resolve(req)
	if err != nil {
		t.Fatalf("expected tenant to resolve, got error: %v", err)
	}
	if tenantID != "greenfield" {
		t.Fatalf("expected greenfield, got %q", tenantID)
	}
}

func TestResolvePathStrategy(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(config.TenantConfig{
		Strategy:   "path",
		PathPrefix: "/t/",
	})

	req := httptest.NewRequest("GET", "http://app.test/t/greenfield/api/v1/schools", nil)
	tenantID, err := resolver.Resolve(req)
	if err != nil {
		t.Fatalf("expected tenant to resolve, got error: %v", err)
	}
	if tenantID != "greenfield" {
		t.Fatalf("expected greenfield, got %q", tenantID)
	}
}

func TestResolvePathStrategyRejectsPartialPrefix(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(config.TenantConfig{
		Strategy:   "path",
		PathPrefix: "/t/",
	})

	req := httptest.NewRequest("GET", "http://app.test/tenant/greenfield/api/v1/schools", nil)
	if _, err := resolver.Resolve(req); err == nil {
		t.Fatal("expected partial prefix path to fail tenant resolution")
	}
}
