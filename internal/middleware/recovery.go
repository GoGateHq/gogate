// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gogatehq/gogate/pkg/response"
)

// recoveryWriter tracks whether any bytes have been written to the client,
// so the recovery handler knows if it is safe to send a 500 JSON response.
type recoveryWriter struct {
	http.ResponseWriter
	written bool
}

func (rw *recoveryWriter) WriteHeader(code int) {
	rw.written = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *recoveryWriter) Write(b []byte) (int, error) {
	rw.written = true
	return rw.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController find Flusher/Hijacker on the inner writer.
func (rw *recoveryWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &recoveryWriter{ResponseWriter: w}

			defer func() {
				if recovered := recover(); recovered != nil {
					requestID := RequestIDFromContext(r.Context())
					logger.Error("panic recovered",
						"request_id", requestID,
						"path", r.URL.Path,
						"method", r.Method,
						"panic", fmt.Sprint(recovered),
					)
					// Only attempt to write an error response if nothing has
					// been sent yet; otherwise the status line is already on
					// the wire and writing again would produce garbage.
					if !rw.written {
						response.Error(rw, http.StatusInternalServerError, "internal server error")
					}
				}
			}()

			next.ServeHTTP(rw, r)
		})
	}
}
