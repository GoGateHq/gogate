// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import "net/http"

// SecurityHeaders adds standard security response headers to every request.
// These headers protect against common web vulnerabilities:
//   - X-Content-Type-Options: nosniff — prevents MIME-type sniffing
//   - X-Frame-Options: DENY — prevents clickjacking via iframes
//   - X-XSS-Protection: 0 — disables legacy XSS auditors (recommended by OWASP)
//   - Referrer-Policy: strict-origin-when-cross-origin — limits referrer leakage
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
