// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gogatehq/gogate/internal/config"
	"github.com/gogatehq/gogate/internal/gateway"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	configPath := defaultConfigPath()
	flag.StringVar(&configPath, "config", configPath, "path to gateway config yaml")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("failed to load config", "path", configPath, "error", err)
		os.Exit(1)
	}

	handler, err := gateway.NewHandler(cfg, logger)
	if err != nil {
		logger.Error("failed to build gateway handler", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:         cfg.Server.ListenAddr(),
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("gateway started", "addr", server.Addr)
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
