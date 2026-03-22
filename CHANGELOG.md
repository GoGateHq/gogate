# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/) and this project follows [Semantic Versioning](https://semver.org/).

## [0.1.0] — 2026-03-22

First public release of GoGate.

### Added

#### Routing & Reverse Proxy
- Config-driven prefix-based routing to any number of upstream services.
- Per-service `strip_prefix` flag to forward clean paths to upstreams (e.g. `/api/v1/users/123` → `/123`).
- Shared HTTP transport with connection pooling (`MaxIdleConns: 200`, `MaxIdleConnsPerHost: 64`).
- Per-service configurable timeout and `max_body_size` limits.
- WebSocket upgrade proxying (Hijack support through all middleware layers).
- Response streaming with `FlushInterval` — no full-body buffering.
- `X-Forwarded-Host` header set on every proxied request.

#### Authentication
- JWT verification with signature, expiry (`exp`), and issuer (`iss`) validation.
- Multi-key support with `kid` header selection for zero-downtime key rotation.
- Optional JWKS endpoint fetch with configurable cache TTL and `singleflight` deduplication.
- Bearer token from `Authorization` header or `?token=` query parameter (WebSocket upgrades).
- Environment variable expansion for secrets in config (`${JWT_SIGNING_KEY}`).
- `Authorization` header stripped before forwarding — upstreams receive trusted `X-User-ID`, `X-User-Roles`, `X-Tenant-ID` headers only.

#### Multi-Tenant Resolution
- Three tenant resolution strategies: `subdomain`, `header`, `path`.
- Per-service `tenant_aware` flag — non-tenant services skip resolution entirely.
- Tenant ID validated against regex pattern (`^[a-z0-9][a-z0-9-]{2,62}$`) to prevent injection.
- Reserved subdomain filtering (`www`, `api`, `admin` configurable).
- JWT `tenant_id` claim cross-checked against resolved tenant — mismatch returns 403.

#### Rate Limiting
- Redis-backed sliding window rate limiter (Lua script, atomic operations).
- Per-service `rate_limit_rpm` with global `default_rpm` fallback.
- Tenant-aware rate limit keys (tenant ID for tenant routes, client IP for non-tenant routes).
- `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset` response headers.
- Configurable `fail_open` mode — allows traffic when Redis is unavailable.

#### Observability
- Prometheus metrics: `gateway_requests_total`, `gateway_request_duration_seconds`, `gateway_upstream_errors_total`.
- Bounded label cardinality — unknown services mapped to `"unknown"`.
- Structured JSON request completion logs with method, path, status, latency, service, request ID.
- `X-Request-ID` injection (UUID v4) with forwarding to upstreams.

#### Security
- Security response headers on every request: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `X-XSS-Protection: 0`, `Referrer-Policy: strict-origin-when-cross-origin`.
- Identity header stripping (`X-User-ID`, `X-Tenant-ID`, `X-User-Roles`, `X-Gateway-Service`) — clients cannot spoof trusted headers.
- CORS middleware with origin allowlist, wildcard + credentials safety check, preflight handling.
- Trusted proxy IP resolution from `X-Forwarded-For` / `X-Real-IP` with CIDR allowlist.
- Panic recovery middleware — returns 500 JSON without crashing the process.

#### Infrastructure
- Single statically-compiled binary (`CGO_ENABLED=0`).
- Build metadata injection via `-ldflags` (version, commit, build date).
- `-version` flag prints build info and exits.
- `-config` flag and `GOGATE_CONFIG` env var for config path.
- Graceful shutdown on SIGINT/SIGTERM with 10-second drain timeout.
- Multi-stage Docker build → `scratch` image (non-root UID 65534).
- `docker-compose.yml` with gateway + Redis + Prometheus for local dev.
- Kubernetes manifests: Deployment, Service, HPA (2–20 pods), security context, probes.
- Example configs for single-tenant and multi-tenant deployments.

#### CI/CD
- GitHub Actions CI: `go mod tidy` check, `vet`, `gofmt`, race-enabled tests, binary build, Docker build.
- CodeQL analysis (Go) on push, PR, and weekly schedule.
- Security scanning: `gosec` with SARIF upload + `govulncheck` on push, PR, and weekly schedule.

### Changed
- **Breaking:** Go module path renamed from `github.com/opportunation/api-gateway` to `github.com/gogatehq/gogate`.

### Migration Notes
- Update imports in downstream projects:
  - from: `github.com/opportunation/api-gateway/...`
  - to: `github.com/gogatehq/gogate/...`
- Run module update commands in consuming repositories:
  - `go get github.com/gogatehq/gogate@latest`
  - `go mod tidy`

### Known Limitations
- Only `HS256` JWT algorithm is currently supported. RS256/ES256 support is planned.
- Config hot-reload is not yet supported — service list and rate limit changes require a gateway restart.
- No circuit breaker — a failing upstream will continue to receive traffic until its timeout is hit.
- No OpenTelemetry tracing — distributed tracing requires manual `X-Request-ID` correlation for now.
