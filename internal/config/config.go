// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig    `yaml:"server"`
	Services []ServiceConfig `yaml:"services"`
}

type ServerConfig struct {
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

type ServiceConfig struct {
	Name   string `yaml:"name"`
	Prefix string `yaml:"prefix"`
	Target string `yaml:"target"`
}

func (s ServerConfig) ListenAddr() string {
	return fmt.Sprintf(":%d", s.Port)
}

func Load(path string) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("config path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", filepath.Clean(path), err)
	}

	var cfg Config
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode yaml: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 30 * time.Second
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 30 * time.Second
	}
	if c.Server.IdleTimeout == 0 {
		c.Server.IdleTimeout = 120 * time.Second
	}
}

func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port)
	}
	if c.Server.ReadTimeout < 0 || c.Server.WriteTimeout < 0 || c.Server.IdleTimeout < 0 {
		return errors.New("server timeouts must not be negative")
	}

	reserved := []string{"/health", "/ready"}
	seenPrefixes := make(map[string]struct{}, len(c.Services))

	for i, svc := range c.Services {
		indexLabel := fmt.Sprintf("services[%d]", i)

		if strings.TrimSpace(svc.Name) == "" {
			return fmt.Errorf("%s.name is required", indexLabel)
		}
		if strings.TrimSpace(svc.Prefix) == "" {
			return fmt.Errorf("%s.prefix is required", indexLabel)
		}
		if !strings.HasPrefix(svc.Prefix, "/") {
			return fmt.Errorf("%s.prefix must start with '/'", indexLabel)
		}
		if slices.Contains(reserved, svc.Prefix) {
			return fmt.Errorf("%s.prefix %q is reserved for system endpoints", indexLabel, svc.Prefix)
		}
		if _, ok := seenPrefixes[svc.Prefix]; ok {
			return fmt.Errorf("%s.prefix %q is duplicated", indexLabel, svc.Prefix)
		}
		seenPrefixes[svc.Prefix] = struct{}{}

		parsed, err := url.Parse(svc.Target)
		if err != nil {
			return fmt.Errorf("%s.target invalid url: %w", indexLabel, err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("%s.target must use http or https scheme", indexLabel)
		}
		if parsed.Host == "" {
			return fmt.Errorf("%s.target host is required", indexLabel)
		}
		if host := parsed.Hostname(); host == "" || strings.Contains(host, " ") {
			return fmt.Errorf("%s.target contains invalid host", indexLabel)
		}
		if port := parsed.Port(); port != "" {
			if _, err := net.LookupPort("tcp", port); err != nil {
				return fmt.Errorf("%s.target has invalid port: %w", indexLabel, err)
			}
		}
	}

	return nil
}
