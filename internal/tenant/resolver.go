// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package tenant

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/gogatehq/gogate/internal/config"
)

var (
	ErrTenantUnresolved = errors.New("tenant could not be resolved")
	ErrTenantInvalid    = errors.New("tenant is invalid")
)

type Resolver struct {
	strategy      string
	headerName    string
	pathPrefix    string
	reservedNames map[string]struct{}
	pattern       *regexp.Regexp
}

func NewResolver(cfg config.TenantConfig) *Resolver {
	reserved := make(map[string]struct{}, len(cfg.ReservedSubdomains))
	for _, name := range cfg.ReservedSubdomains {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if normalized == "" {
			continue
		}
		reserved[normalized] = struct{}{}
	}

	return &Resolver{
		strategy:      cfg.Strategy,
		headerName:    cfg.HeaderName,
		pathPrefix:    cfg.PathPrefix,
		reservedNames: reserved,
		pattern:       regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{2,62}$`),
	}
}

func (r *Resolver) Resolve(req *http.Request) (string, error) {
	var tenantID string
	switch r.strategy {
	case "subdomain":
		tenantID = resolveSubdomain(req.Host)
	case "header":
		tenantID = strings.TrimSpace(req.Header.Get(r.headerName))
	case "path":
		tenantID = resolvePath(req.URL.Path, r.pathPrefix)
	default:
		return "", fmt.Errorf("%w: unsupported strategy %q", ErrTenantUnresolved, r.strategy)
	}

	tenantID = strings.ToLower(strings.TrimSpace(tenantID))
	if tenantID == "" {
		return "", ErrTenantUnresolved
	}
	if _, reserved := r.reservedNames[tenantID]; reserved {
		return "", ErrTenantUnresolved
	}
	if !r.pattern.MatchString(tenantID) {
		return "", ErrTenantInvalid
	}
	return tenantID, nil
}

func resolveSubdomain(host string) string {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return ""
	}

	hostname := trimmed
	if parsedHost, _, err := net.SplitHostPort(trimmed); err == nil {
		hostname = parsedHost
	}

	parts := strings.Split(hostname, ".")
	if len(parts) < 3 {
		return ""
	}
	return parts[0]
}

func resolvePath(path string, pathPrefix string) string {
	normalizedPrefix := strings.TrimSuffix(strings.TrimSpace(pathPrefix), "/")
	if normalizedPrefix == "" {
		return ""
	}
	if !(path == normalizedPrefix || strings.HasPrefix(path, normalizedPrefix+"/")) {
		return ""
	}

	remainder := strings.TrimPrefix(path, normalizedPrefix)
	remainder = strings.TrimPrefix(remainder, "/")
	if remainder == "" {
		return ""
	}

	segment := strings.SplitN(remainder, "/", 2)[0]
	return strings.TrimSpace(segment)
}
