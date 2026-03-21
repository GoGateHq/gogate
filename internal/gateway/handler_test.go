// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package gateway

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestResolveClientIPUntrustedProxyIgnoresForwardedHeaders(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("GET", "http://example.test/path", nil)
	req.RemoteAddr = "198.51.100.10:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.Header.Set("X-Real-IP", "203.0.113.11")

	ip := resolveClientIP(req, []netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"),
	})
	if ip != "198.51.100.10" {
		t.Fatalf("expected remote addr IP when untrusted, got %q", ip)
	}
}

func TestResolveClientIPTrustedProxyUsesForwardedHeaders(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("GET", "http://example.test/path", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 127.0.0.1")
	req.Header.Set("X-Real-IP", "203.0.113.11")

	ip := resolveClientIP(req, []netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"),
	})
	if ip != "203.0.113.10" {
		t.Fatalf("expected first forwarded-for IP, got %q", ip)
	}
}
