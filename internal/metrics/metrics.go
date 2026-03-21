// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Gateway holds all Prometheus metrics for the API gateway.
type Gateway struct {
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	UpstreamErrors  *prometheus.CounterVec
}

// New creates and registers gateway metrics on the given registerer.
func New(reg prometheus.Registerer) *Gateway {
	m := &Gateway{
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gateway",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests processed by the gateway.",
		}, []string{"service", "method", "status"}),

		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "gateway",
			Name:      "request_duration_seconds",
			Help:      "Histogram of request latencies in seconds.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"service", "method", "status"}),

		UpstreamErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gateway",
			Name:      "upstream_errors_total",
			Help:      "Total number of upstream proxy errors.",
		}, []string{"service"}),
	}

	reg.MustRegister(m.RequestsTotal, m.RequestDuration, m.UpstreamErrors)
	return m
}
