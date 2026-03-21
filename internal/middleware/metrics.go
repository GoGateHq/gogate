// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gogatehq/gogate/internal/metrics"
)

// Metrics returns middleware that records Prometheus metrics for every request.
// knownServices is the allowlist of valid service names; any value not in this
// set is mapped to "unknown" to prevent unbounded label cardinality.
func Metrics(m *metrics.Gateway, knownServices []string) func(http.Handler) http.Handler {
	if m == nil {
		return func(next http.Handler) http.Handler { return next }
	}

	allowed := make(map[string]struct{}, len(knownServices))
	for _, s := range knownServices {
		allowed[s] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &metricsRecorder{ResponseWriter: w, status: 0}

			next.ServeHTTP(rec, r)

			if rec.status == 0 {
				rec.status = http.StatusOK
			}

			service := r.Header.Get("X-Gateway-Service")
			if _, ok := allowed[service]; !ok {
				service = "unknown"
			}
			status := strconv.Itoa(rec.status)
			duration := time.Since(start).Seconds()

			m.RequestsTotal.WithLabelValues(service, r.Method, status).Inc()
			m.RequestDuration.WithLabelValues(service, r.Method, status).Observe(duration)
		})
	}
}

type metricsRecorder struct {
	http.ResponseWriter
	status int
}

func (m *metricsRecorder) WriteHeader(code int) {
	m.status = code
	m.ResponseWriter.WriteHeader(code)
}

func (m *metricsRecorder) Write(b []byte) (int, error) {
	if m.status == 0 {
		m.status = http.StatusOK
	}
	return m.ResponseWriter.Write(b)
}

func (m *metricsRecorder) Unwrap() http.ResponseWriter {
	return m.ResponseWriter
}
