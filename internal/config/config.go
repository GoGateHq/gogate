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

var tenantIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{2,62}$`)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	JWT       JWTConfig       `yaml:"jwt"`
	Tenant    TenantConfig    `yaml:"tenant"`
	CORS      CORSConfig      `yaml:"cors"`
	Redis     RedisConfig     `yaml:"redis"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Metrics   MetricsConfig   `yaml:"metrics"`
	Services  []ServiceConfig `yaml:"services"`
}

type ServerConfig struct {
	Port           int           `yaml:"port"`
	ReadTimeout    time.Duration `yaml:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout"`
	IdleTimeout    time.Duration `yaml:"idle_timeout"`
	TrustedProxies []string      `yaml:"trusted_proxies"`
}

type ServiceConfig struct {
	Name         string        `yaml:"name"`
	Prefix       string        `yaml:"prefix"`
	Target       string        `yaml:"target"`
	SkipAuth     *bool         `yaml:"skip_auth"`
	TenantAware  *bool         `yaml:"tenant_aware"`
	StripPrefix  *bool         `yaml:"strip_prefix"`
	Timeout      time.Duration `yaml:"timeout"`
	MaxBodySize  int64         `yaml:"max_body_size"`
	RateLimitRPM *int          `yaml:"rate_limit_rpm"`
}

type RedisConfig struct {
	Addr         string        `yaml:"addr"`
	Password     string        `yaml:"password"`
	DB           int           `yaml:"db"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

type RateLimitConfig struct {
	DefaultRPM int    `yaml:"default_rpm"`
	FailOpen   *bool  `yaml:"fail_open"`
	KeyPrefix  string `yaml:"key_prefix"`
}

type MetricsConfig struct {
	Enabled *bool  `yaml:"enabled"`
	Path    string `yaml:"path"`
}

func (r RateLimitConfig) IsFailOpen() bool {
	if r.FailOpen == nil {
		return true
	}
	return *r.FailOpen
}

func (m MetricsConfig) IsEnabled() bool {
	if m.Enabled == nil {
		return false
	}
	return *m.Enabled
}

func (m MetricsConfig) EffectivePath() string {
	if strings.TrimSpace(m.Path) == "" {
		return "/metrics"
	}
	return m.Path
}

func (s ServiceConfig) EffectiveRPM(fallback int) int {
	if s.RateLimitRPM != nil {
		return *s.RateLimitRPM
	}
	return fallback
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

type CORSConfig struct {
	AllowedOrigins   []string `yaml:"allowed_origins"`
	AllowedMethods   []string `yaml:"allowed_methods"`
	AllowedHeaders   []string `yaml:"allowed_headers"`
	ExposedHeaders   []string `yaml:"exposed_headers"`
	AllowCredentials bool     `yaml:"allow_credentials"`
	MaxAge           int      `yaml:"max_age"`
}

func (s ServerConfig) ListenAddr() string {
	return fmt.Sprintf(":%d", s.Port)
}

func (s ServiceConfig) IsAuthSkipped() bool {
	// Secure default: require auth when field is omitted.
	if s.SkipAuth == nil {
		return false
	}
	return *s.SkipAuth
}

func (s ServiceConfig) IsTenantAware() bool {
	if s.TenantAware == nil {
		return false
	}
	return *s.TenantAware
}

func (s ServiceConfig) IsPrefixStripped() bool {
	if s.StripPrefix == nil {
		return false
	}
	return *s.StripPrefix
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
	if c.Redis.DialTimeout == 0 {
		c.Redis.DialTimeout = 5 * time.Second
	}
	if c.Redis.ReadTimeout == 0 {
		c.Redis.ReadTimeout = 3 * time.Second
	}
	if c.Redis.WriteTimeout == 0 {
		c.Redis.WriteTimeout = 3 * time.Second
	}
	if strings.TrimSpace(c.RateLimit.KeyPrefix) == "" {
		c.RateLimit.KeyPrefix = "gogate:rl:"
	}
	if strings.TrimSpace(c.Metrics.Path) == "" {
		c.Metrics.Path = "/metrics"
	}
	if len(c.CORS.AllowedOrigins) > 0 {
		if len(c.CORS.AllowedMethods) == 0 {
			c.CORS.AllowedMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
		}
		if len(c.CORS.AllowedHeaders) == 0 {
			c.CORS.AllowedHeaders = []string{"Authorization", "Content-Type", "X-Request-ID", "X-Tenant-ID"}
		}
		if len(c.CORS.ExposedHeaders) == 0 {
			c.CORS.ExposedHeaders = []string{"X-Request-ID"}
		}
		if c.CORS.MaxAge == 0 {
			c.CORS.MaxAge = 86400
		}
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
	if c.Metrics.IsEnabled() {
		reserved = append(reserved, c.Metrics.EffectivePath())
	}
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

		if svc.Timeout < 0 {
			return fmt.Errorf("%s.timeout must not be negative", indexLabel)
		}
		if svc.MaxBodySize < 0 {
			return fmt.Errorf("%s.max_body_size must not be negative", indexLabel)
		}
		if svc.RateLimitRPM != nil && *svc.RateLimitRPM < 0 {
			return fmt.Errorf("%s.rate_limit_rpm must not be negative", indexLabel)
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
	if err := c.validateRateLimit(); err != nil {
		return err
	}
	if err := c.validateMetrics(); err != nil {
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

	for _, reserved := range c.Tenant.ReservedSubdomains {
		if !tenantIDPattern.MatchString(strings.ToLower(strings.TrimSpace(reserved))) {
			return fmt.Errorf("tenant.reserved_subdomains contains invalid value %q", reserved)
		}
	}

	return nil
}

func (c *Config) requiresRateLimit() bool {
	if c.RateLimit.DefaultRPM > 0 {
		return true
	}
	for _, svc := range c.Services {
		if svc.RateLimitRPM != nil && *svc.RateLimitRPM > 0 {
			return true
		}
	}
	return false
}

func (c *Config) validateRateLimit() error {
	if c.RateLimit.DefaultRPM < 0 {
		return errors.New("rate_limit.default_rpm must not be negative")
	}
	if c.requiresRateLimit() && strings.TrimSpace(c.Redis.Addr) == "" {
		return errors.New("redis.addr is required when rate limiting is enabled")
	}
	return nil
}

func (c *Config) validateMetrics() error {
	if !c.Metrics.IsEnabled() {
		return nil
	}
	path := c.Metrics.EffectivePath()
	if !strings.HasPrefix(path, "/") {
		return errors.New("metrics.path must start with '/'")
	}
	return nil
}
