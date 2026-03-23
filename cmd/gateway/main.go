// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/gogatehq/gogate/internal/config"
	"github.com/gogatehq/gogate/internal/gateway"
	"github.com/gogatehq/gogate/internal/metrics"
	"github.com/gogatehq/gogate/internal/ratelimit"
)

// Set via -ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	showVersion := false
	configPath := defaultConfigPath()
	flag.StringVar(&configPath, "config", configPath, "path to gateway config yaml")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()

	if showVersion {
		fmt.Printf("gogate %s (commit=%s, built=%s)\n", version, commit, date)
		os.Exit(0)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("failed to load config", "path", configPath, "error", err)
		os.Exit(1)
	}

	deps := &gateway.HandlerDeps{}

	// Rate limiter (optional).
	if strings.TrimSpace(cfg.Redis.Addr) != "" {
		limiter, err := ratelimit.NewLimiter(cfg.Redis, cfg.RateLimit, logger)
		if err != nil {
			logger.Error("failed to connect to redis", "error", err)
			os.Exit(1)
		}
		deps.Limiter = limiter
		defer limiter.Close()
		logger.Info("rate limiter enabled", "redis", cfg.Redis.Addr, "default_rpm", cfg.RateLimit.DefaultRPM)
	}

	// Prometheus metrics (optional).
	if cfg.Metrics.IsEnabled() {
		registry := prometheus.NewRegistry()
		registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
		registry.MustRegister(prometheus.NewGoCollector())
		deps.Metrics = metrics.New(registry)
		deps.Registry = registry
		logger.Info("metrics enabled", "path", cfg.Metrics.EffectivePath())
	}

	handler, err := gateway.NewHandler(cfg, logger, deps)
	if err != nil {
		logger.Error("failed to build gateway handler", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              cfg.Server.ListenAddr(),
		Handler:           handler,
		ReadTimeout:       cfg.Server.ReadTimeout,
		ReadHeaderTimeout: cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("gateway started", "addr", server.Addr, "version", version)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("gateway server crashed", "error", err)
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	var runErr error
	select {
	case <-stop:
	case runErr = <-errCh:
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer signal.Stop(stop)

	logger.Info("shutting down gateway")
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("shutdown failed", "error", err)
		if runErr == nil {
			runErr = err
		}
	}

	if runErr != nil {
		os.Exit(1)
	}
}

func defaultConfigPath() string {
	if path := os.Getenv("GOGATE_CONFIG"); path != "" {
		return path
	}
	return "config.yaml"
}
