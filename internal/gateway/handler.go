// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package gateway

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/gogatehq/gogate/internal/auth"
	"github.com/gogatehq/gogate/internal/config"
	"github.com/gogatehq/gogate/internal/middleware"
	"github.com/gogatehq/gogate/internal/tenant"
	"github.com/gogatehq/gogate/pkg/response"
)

type routeProxy struct {
	name        string
	prefix      string
	proxy       *httputil.ReverseProxy
	skipAuth    bool
	tenantAware bool
}

type healthBody struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

func NewHandler(cfg *config.Config, logger *slog.Logger) (http.Handler, error) {
	if logger == nil {
		logger = slog.Default()
	}

	authVerifier := auth.NewVerifier(cfg.JWT)
	tenantResolver := tenant.NewResolver(cfg.Tenant)
	trustedProxies, err := parseTrustedProxyPrefixes(cfg.Server.TrustedProxies)
	if err != nil {
		return nil, err
	}

	routes := make([]routeProxy, 0, len(cfg.Services))
	for _, svc := range cfg.Services {
		targetURL, err := url.Parse(svc.Target)
		if err != nil {
			return nil, err
		}

		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		proxy.Transport = &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          200,
			MaxIdleConnsPerHost:   64,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
		}
		proxy.FlushInterval = 50 * time.Millisecond
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			status := http.StatusServiceUnavailable
			if isTimeoutError(err) || errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}

			logger.Error("upstream proxy error",
				"request_id", middleware.RequestIDFromContext(r.Context()),
				"error", err,
				"path", r.URL.Path,
				"method", r.Method,
			)
			response.Error(w, status, "upstream unavailable")
		}

		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalHost := req.Host
			originalDirector(req)
			req.Header.Set("X-Forwarded-Host", originalHost)
		}

		routes = append(routes, routeProxy{
			name:        svc.Name,
			prefix:      svc.Prefix,
			proxy:       proxy,
			skipAuth:    svc.IsAuthSkipped(),
			tenantAware: svc.IsTenantAware(),
		})
	}

	sort.SliceStable(routes, func(i, j int) bool {
		return len(routes[i].prefix) > len(routes[j].prefix)
	})

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recovery(logger))
	router.Use(middleware.Logging(logger))

	router.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		response.JSON(w, http.StatusOK, healthBody{
			Status:  "ok",
			Service: "api-gateway",
		})
	})

	router.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		response.JSON(w, http.StatusOK, healthBody{
			Status:  "ok",
			Service: "api-gateway",
		})
	})

	router.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		clientIP := resolveClientIP(r, trustedProxies)
		if clientIP != "" {
			r.RemoteAddr = net.JoinHostPort(clientIP, "0")
			r.Header.Set("X-Real-IP", clientIP)
		}

		for _, route := range routes {
			if matchesPrefix(r.URL.Path, route.prefix) {
				if !prepareRouteRequest(w, r, route, authVerifier, tenantResolver) {
					return
				}

				r.Header.Set("X-Gateway-Service", route.name)
				route.proxy.ServeHTTP(w, r)
				return
			}
		}

		response.Error(w, http.StatusNotFound, "route not found")
	})

	return router, nil
}

func matchesPrefix(path string, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	if len(path) == len(prefix) || strings.HasSuffix(prefix, "/") {
		return true
	}
	return path[len(prefix)] == '/'
}

func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func prepareRouteRequest(
	w http.ResponseWriter,
	r *http.Request,
	route routeProxy,
	authVerifier *auth.Verifier,
	tenantResolver *tenant.Resolver,
) bool {
	stripIdentityHeaders(r.Header)

	resolvedTenantID := ""
	if route.tenantAware {
		tenantID, err := tenantResolver.Resolve(r)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "tenant could not be resolved")
			return false
		}
		resolvedTenantID = tenantID
		r.Header.Set("X-Tenant-ID", tenantID)
	}

	if route.skipAuth {
		return true
	}

	authorizationHeader := r.Header.Get("Authorization")
	if authorizationHeader == "" && isWebSocketUpgrade(r) {
		if queryToken := strings.TrimSpace(r.URL.Query().Get("token")); queryToken != "" {
			authorizationHeader = "Bearer " + queryToken
		}
	}

	identity, err := authVerifier.Verify(r.Context(), authorizationHeader)
	if err != nil {
		response.Error(w, http.StatusUnauthorized, "invalid or missing authentication token")
		return false
	}
	if strings.TrimSpace(identity.UserID) == "" {
		response.Error(w, http.StatusUnauthorized, "invalid authentication claims")
		return false
	}

	if route.tenantAware {
		if strings.TrimSpace(identity.TenantID) == "" || !strings.EqualFold(identity.TenantID, resolvedTenantID) {
			response.Error(w, http.StatusForbidden, "tenant mismatch")
			return false
		}
	}

	r.Header.Set("X-User-ID", identity.UserID)
	if len(identity.Roles) > 0 {
		r.Header.Set("X-User-Roles", strings.Join(identity.Roles, ","))
	}
	if !route.tenantAware && strings.TrimSpace(identity.TenantID) != "" {
		r.Header.Set("X-Tenant-ID", identity.TenantID)
	}

	return true
}

func stripIdentityHeaders(headers http.Header) {
	headers.Del("X-User-ID")
	headers.Del("X-Tenant-ID")
	headers.Del("X-User-Roles")
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket")
}

func parseTrustedProxyPrefixes(values []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil {
			return nil, err
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

func resolveClientIP(r *http.Request, trustedProxies []netip.Prefix) string {
	remoteIP := parseRemoteIP(r.RemoteAddr)
	if remoteIP == "" {
		return ""
	}
	if !isTrustedProxy(remoteIP, trustedProxies) {
		return remoteIP
	}

	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		candidate := strings.TrimSpace(strings.Split(forwarded, ",")[0])
		if parseRemoteIP(candidate) != "" {
			return parseRemoteIP(candidate)
		}
	}
	if realIP := parseRemoteIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); realIP != "" {
		return realIP
	}
	return remoteIP
}

func parseRemoteIP(remoteAddr string) string {
	trimmed := strings.TrimSpace(remoteAddr)
	if trimmed == "" {
		return ""
	}

	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		trimmed = host
	}
	addr, err := netip.ParseAddr(trimmed)
	if err != nil {
		return ""
	}
	return addr.Unmap().String()
}

func isTrustedProxy(ip string, trustedProxies []netip.Prefix) bool {
	if len(trustedProxies) == 0 {
		return false
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, prefix := range trustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
