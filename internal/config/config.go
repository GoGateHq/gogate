// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig    `yaml:"server"`
	JWT      JWTConfig       `yaml:"jwt"`
	Tenant   TenantConfig    `yaml:"tenant"`
	Services []ServiceConfig `yaml:"services"`
}

type ServerConfig struct {
	Port           int           `yaml:"port"`
	ReadTimeout    time.Duration `yaml:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout"`
	IdleTimeout    time.Duration `yaml:"idle_timeout"`
	TrustedProxies []string      `yaml:"trusted_proxies"`
}

type ServiceConfig struct {
	Name        string `yaml:"name"`
	Prefix      string `yaml:"prefix"`
	Target      string `yaml:"target"`
	SkipAuth    *bool  `yaml:"skip_auth"`
	TenantAware *bool  `yaml:"tenant_aware"`
}

type JWTConfig struct {
	Issuer       string         `yaml:"issuer"`
	Algorithms   []string       `yaml:"algorithms"`
	ClockSkew    time.Duration  `yaml:"clock_skew"`
	Keys         []JWTKeyConfig `yaml:"keys"`
	JWKSURL      string         `yaml:"jwks_url"`
	JWKSCacheTTL time.Duration  `yaml:"jwks_cache_ttl"`
}

type JWTKeyConfig struct {
	KID     string `yaml:"kid"`
	KTY     string `yaml:"kty"`
	Value   string `yaml:"value"`
	Primary bool   `yaml:"primary"`
}

type TenantConfig struct {
	Strategy           string   `yaml:"strategy"`
	HeaderName         string   `yaml:"header_name"`
	PathPrefix         string   `yaml:"path_prefix"`
	ReservedSubdomains []string `yaml:"reserved_subdomains"`
}

func (s ServerConfig) ListenAddr() string {
	return fmt.Sprintf(":%d", s.Port)
}

func (s ServiceConfig) IsAuthSkipped() bool {
	// Keep v1 behavior by defaulting to skip auth when field is omitted.
	if s.SkipAuth == nil {
		return true
	}
	return *s.SkipAuth
}

func (s ServiceConfig) IsTenantAware() bool {
	if s.TenantAware == nil {
		return false
	}
	return *s.TenantAware
}

func Load(path string) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("config path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", filepath.Clean(path), err)
	}

	expandedData := os.ExpandEnv(string(data))

	var cfg Config
	decoder := yaml.NewDecoder(strings.NewReader(expandedData))
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
	if len(c.JWT.Algorithms) == 0 {
		c.JWT.Algorithms = []string{"HS256"}
	}
	if c.JWT.ClockSkew == 0 {
		c.JWT.ClockSkew = 30 * time.Second
	}
	if c.JWT.JWKSCacheTTL == 0 {
		c.JWT.JWKSCacheTTL = 5 * time.Minute
	}
	if strings.TrimSpace(c.Tenant.Strategy) == "" {
		c.Tenant.Strategy = "subdomain"
	}
	if strings.TrimSpace(c.Tenant.HeaderName) == "" && strings.TrimSpace(c.Tenant.Strategy) != "header" {
		c.Tenant.HeaderName = "X-Tenant-ID"
	}
	if strings.TrimSpace(c.Tenant.PathPrefix) == "" {
		c.Tenant.PathPrefix = "/t/"
	}
	if len(c.Tenant.ReservedSubdomains) == 0 {
		c.Tenant.ReservedSubdomains = []string{"www", "api", "admin"}
	}
	for i := range c.JWT.Keys {
		if strings.TrimSpace(c.JWT.Keys[i].KTY) == "" {
			c.JWT.Keys[i].KTY = "oct"
		}
	}
}

func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port)
	}
	if c.Server.ReadTimeout < 0 || c.Server.WriteTimeout < 0 || c.Server.IdleTimeout < 0 {
		return errors.New("server timeouts must not be negative")
	}
	for _, cidr := range c.Server.TrustedProxies {
		if _, err := netip.ParsePrefix(strings.TrimSpace(cidr)); err != nil {
			return fmt.Errorf("server.trusted_proxies contains invalid CIDR %q: %w", cidr, err)
		}
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

		if svc.IsTenantAware() && c.Tenant.Strategy == "header" && strings.TrimSpace(c.Tenant.HeaderName) == "" {
			return errors.New("tenant.header_name is required when tenant.strategy=header and tenant_aware routes exist")
		}
	}

	if c.requiresAuth() && len(c.JWT.Keys) == 0 && strings.TrimSpace(c.JWT.JWKSURL) == "" {
		return errors.New("jwt.keys or jwt.jwks_url is required when auth-protected routes exist")
	}

	if err := c.validateJWT(); err != nil {
		return err
	}
	if err := c.validateTenant(); err != nil {
		return err
	}

	return nil
}

func (c *Config) validateJWT() error {
	allowed := map[string]struct{}{
		"HS256": {},
	}
	for _, alg := range c.JWT.Algorithms {
		if _, ok := allowed[alg]; !ok {
			return fmt.Errorf("jwt.algorithms contains unsupported value %q", alg)
		}
	}
	if c.JWT.ClockSkew < 0 {
		return errors.New("jwt.clock_skew must not be negative")
	}
	if c.JWT.JWKSCacheTTL < 0 {
		return errors.New("jwt.jwks_cache_ttl must not be negative")
	}

	if c.JWT.JWKSURL != "" {
		parsed, err := url.Parse(c.JWT.JWKSURL)
		if err != nil {
			return fmt.Errorf("jwt.jwks_url invalid url: %w", err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("jwt.jwks_url must use http or https")
		}
		if parsed.Host == "" {
			return errors.New("jwt.jwks_url host is required")
		}
	}

	seenKIDs := make(map[string]struct{}, len(c.JWT.Keys))
	primaryCount := 0
	for i, key := range c.JWT.Keys {
		indexLabel := fmt.Sprintf("jwt.keys[%d]", i)
		if strings.TrimSpace(key.KID) == "" {
			return fmt.Errorf("%s.kid is required", indexLabel)
		}
		if _, ok := seenKIDs[key.KID]; ok {
			return fmt.Errorf("%s.kid %q is duplicated", indexLabel, key.KID)
		}
		seenKIDs[key.KID] = struct{}{}
		if key.Primary {
			primaryCount++
		}
		if key.KTY != "oct" {
			return fmt.Errorf("%s.kty %q is unsupported; only oct is currently supported", indexLabel, key.KTY)
		}
		if strings.TrimSpace(key.Value) == "" {
			return fmt.Errorf("%s.value is required", indexLabel)
		}
	}
	if primaryCount > 1 {
		return errors.New("jwt.keys has multiple primary keys")
	}

	return nil
}

func (c *Config) requiresAuth() bool {
	for _, svc := range c.Services {
		if !svc.IsAuthSkipped() {
			return true
		}
	}
	return false
}

func (c *Config) validateTenant() error {
	switch c.Tenant.Strategy {
	case "subdomain", "header", "path":
	default:
		return fmt.Errorf("tenant.strategy must be one of subdomain|header|path, got %q", c.Tenant.Strategy)
	}

	if c.Tenant.Strategy == "header" && strings.TrimSpace(c.Tenant.HeaderName) == "" {
		return errors.New("tenant.header_name is required when tenant.strategy=header")
	}
	if c.Tenant.Strategy == "path" {
		if !strings.HasPrefix(c.Tenant.PathPrefix, "/") {
			return errors.New("tenant.path_prefix must start with '/'")
		}
		if !strings.HasSuffix(c.Tenant.PathPrefix, "/") {
			return errors.New("tenant.path_prefix must end with '/'")
		}
	}

	tenantPattern := regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{2,62}$`)
	for _, reserved := range c.Tenant.ReservedSubdomains {
		if !tenantPattern.MatchString(strings.ToLower(strings.TrimSpace(reserved))) {
			return fmt.Errorf("tenant.reserved_subdomains contains invalid value %q", reserved)
		}
	}

	return nil
}
