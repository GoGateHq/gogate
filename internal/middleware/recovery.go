// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gogatehq/gogate/pkg/response"
)

func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					requestID := RequestIDFromContext(r.Context())
					logger.Error("panic recovered",
						"request_id", requestID,
						"path", r.URL.Path,
						"method", r.Method,
						"panic", fmt.Sprint(recovered),
					)
					response.Error(w, http.StatusInternalServerError, "internal server error")
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
