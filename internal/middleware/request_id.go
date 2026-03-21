// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/gogatehq/gogate/pkg/contextkeys"
)

const requestIDHeader = "X-Request-ID"

// maxRequestIDLen prevents clients from sending arbitrarily long request IDs.
const maxRequestIDLen = 128

// requestIDPattern validates that a client-supplied request ID contains only
// safe characters (alphanumeric, hyphens, and underscores).
var requestIDPattern = regexp.MustCompile(`^[a-zA-Z0-9\-_]{1,128}$`)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get(requestIDHeader))
		if requestID == "" || len(requestID) > maxRequestIDLen || !requestIDPattern.MatchString(requestID) {
			requestID = uuid.NewString()
		}

		ctx := context.WithValue(r.Context(), contextkeys.RequestID, requestID)
		r = r.WithContext(ctx)
		r.Header.Set(requestIDHeader, requestID)
		w.Header().Set(requestIDHeader, requestID)

		next.ServeHTTP(w, r)
	})
}

func RequestIDFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(contextkeys.RequestID).(string); ok {
		return value
	}
	return ""
}
