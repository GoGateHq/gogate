// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gogatehq/gogate/internal/config"
)

// CORS returns middleware that handles Cross-Origin Resource Sharing.
// If no allowed origins are configured, the middleware is a no-op.
func CORS(cfg config.CORSConfig) func(http.Handler) http.Handler {
	if len(cfg.AllowedOrigins) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	allowAll := len(cfg.AllowedOrigins) == 1 && cfg.AllowedOrigins[0] == "*"

	// Wildcard origins with credentials is insecure: browsers will reject
	// it, and echoing arbitrary origins with credentials leaks auth cookies.
	// Silently disable credentials in this case to prevent misconfiguration.
	if allowAll && cfg.AllowCredentials {
		cfg.AllowCredentials = false
	}

	originsSet := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		originsSet[strings.ToLower(o)] = struct{}{}
	}

	methods := strings.Join(cfg.AllowedMethods, ", ")
	headers := strings.Join(cfg.AllowedHeaders, ", ")
	exposed := strings.Join(cfg.ExposedHeaders, ", ")
	maxAge := strconv.Itoa(cfg.MaxAge)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			if !allowAll {
				if _, ok := originsSet[strings.ToLower(origin)]; !ok {
					next.ServeHTTP(w, r)
					return
				}
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)
			if exposed != "" {
				w.Header().Set("Access-Control-Expose-Headers", exposed)
			}
			if cfg.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Add("Vary", "Origin")

			// Handle preflight requests.
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", methods)
				w.Header().Set("Access-Control-Allow-Headers", headers)
				if maxAge != "0" {
					w.Header().Set("Access-Control-Max-Age", maxAge)
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
