# Product Requirements Document
## GoGate — Enterprise API Gateway

| Field | Value |
|---|---|
| **Document Version** | 1.2.0 |
| **Status** | Draft |
| **Author** | Engineering Team |
| **Created** | 2026-03-20 |
| **Last Updated** | 2026-03-20 (Revision 1.2) |
| **Classification** | Public Draft — Open Source |

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Problem Statement](#2-problem-statement)
3. [Goals and Non-Goals](#3-goals-and-non-goals)
4. [Stakeholders](#4-stakeholders)
5. [User Personas](#5-user-personas)
6. [System Architecture Overview](#6-system-architecture-overview)
7. [Functional Requirements](#7-functional-requirements)
8. [Non-Functional Requirements](#8-non-functional-requirements)
9. [Technical Specifications](#9-technical-specifications)
10. [Data Models](#10-data-models)
11. [API Specification](#11-api-specification)
12. [Security Requirements](#12-security-requirements)
13. [Observability and Monitoring](#13-observability-and-monitoring)
14. [Deployment and Infrastructure](#14-deployment-and-infrastructure)
15. [Testing Strategy](#15-testing-strategy)
16. [Migration and Rollout Plan](#16-migration-and-rollout-plan)
17. [Risks and Mitigations](#17-risks-and-mitigations)
18. [Success Metrics and KPIs](#18-success-metrics-and-kpis)
19. [Future Roadmap](#19-future-roadmap)
20. [Glossary](#20-glossary)
21. [Appendix](#21-appendix)

---

## 1. Executive Summary

GoGate is a self-hosted API Gateway built in Go, designed to serve as the unified entry point for all backend services across one or more software products. It is distributed under an open-source community license with an optional commercial license for use cases requiring non-open-source distribution terms. It is purpose-built for multi-tenant SaaS platforms but fully supports single-tenant and non-tenant applications through configuration flags — with zero code changes required between project types.

GoGate handles cross-cutting concerns — authentication, authorization, rate limiting, request routing, observability, and tenant resolution — at the network edge before requests ever reach a downstream microservice. This removes the responsibility of re-implementing these concerns in every individual service, resulting in leaner services, consistent security policies, and a single plane of control for the entire system.

The gateway is implemented as a single statically-compiled binary, deployable as a Docker container or Kubernetes pod, and configured entirely via a YAML file with environment variable overrides for secrets. It is designed to be reused across multiple independent projects with no modifications to the binary — only the configuration file changes.

---

## 2. Problem Statement

### 2.1 Current Landscape

Modern backend systems are composed of many independent services — auth, billing, notifications, core domain services, and so on. Without a centralized gateway layer, every one of these services must independently:

- Validate JWT tokens and parse claims
- Resolve which tenant a request belongs to
- Enforce rate limits per client or tenant
- Log request and response metadata
- Handle CORS policies
- Expose health check endpoints

This leads to duplicated code across services, inconsistent enforcement of security policies, and scattered observability data that makes debugging across service boundaries difficult.

### 2.2 Pain Points Addressed

| Pain Point | Impact Without Gateway | Impact With GoGate |
|---|---|---|
| JWT validation in every service | Repeated code, risk of inconsistent logic | Validated once at the edge |
| Tenant resolution scattered | Each service may resolve differently | Single canonical strategy |
| Rate limiting per-service | Inconsistent limits, hard to enforce | Centrally configured, Redis-backed |
| No unified request tracing | Hard to correlate logs across services | Request ID injected and forwarded |
| CORS configured per service | Misconfigurations cause frontend errors | Configured once at the gateway |
| Observability data scattered | Requires querying each service | Single Prometheus endpoint covers all |
| Deploying new services | Requires updating multiple configs | One gateway config entry |

### 2.3 Why Build Rather Than Buy

Existing commercial API gateways (AWS API Gateway, Kong, Apigee) solve these problems but introduce their own constraints: licensing costs, vendor lock-in, opaque runtime behaviour, and limited customisability for per-tenant routing logic specific to a multi-tenant SaaS architecture. GoGate is purpose-built for this use case, owned entirely, and extensible without external dependencies.

---

## 3. Goals and Non-Goals

### 3.1 Goals

- **G1** — Provide a single, unified entry point for all HTTP/HTTPS traffic across all backend services.
- **G2** — Support multi-tenant SaaS routing by resolving tenant identity from the request (subdomain, header, or path) and injecting it into downstream context.
- **G3** — Validate JWT tokens once at the gateway and forward trusted user context headers to downstream services so they do not need to re-validate.
- **G4** — Enforce per-tenant, per-service rate limits using a Redis-backed sliding window algorithm.
- **G5** — Be entirely config-driven so that adding a new service requires only a YAML configuration change, not a code deployment.
- **G6** — Be reusable across multiple unrelated projects (multi-tenant SaaS, single-tenant apps, internal tools) through the same binary with project-specific config files.
- **G7** — Expose structured Prometheus metrics and structured JSON logs for every request passing through the gateway.
- **G8** — Support graceful shutdown to drain in-flight requests without dropping them.
- **G9** — Be deployable as a single Docker container with no external runtime dependencies other than Redis.
- **G10** — Achieve sub-5ms median overhead (p50 gateway processing time, excluding network and upstream latency) under normal production load.

### 3.2 Non-Goals

- **NG1** — GoGate is not a service mesh (it does not handle east-west, service-to-service traffic).
- **NG2** — GoGate does not manage TLS certificates (TLS termination is handled upstream by a load balancer or Ingress controller).
- **NG3** — GoGate does not store request or response bodies (it is not an audit log solution).
- **NG4** — GoGate does not perform API schema validation (e.g. OpenAPI request body validation against a spec).
- **NG5** — GoGate does not implement business logic or domain-specific routing rules beyond prefix matching.
- **NG6** — GoGate does not replace a service mesh for mutual TLS or certificate-based service identity between internal services.
- **NG7** — GoGate does not provide a web-based management UI in v1.0.

---

## 4. Stakeholders

| Role | Name / Team | Responsibility |
|---|---|---|
| Product Owner | Engineering Lead | Prioritisation, acceptance criteria |
| Backend Engineer | Core Team | Implementation, code review |
| DevOps / Platform | Infrastructure Team | Deployment, Kubernetes, CI/CD |
| Security | Security Team | Review of auth, rate limiting, CORS policies |
| Frontend Team | Client Applications | Consumer of downstream APIs routed through gateway |
| QA | Test Team | Test plan execution, regression |

---

## 5. User Personas

### 5.1 Backend Service Developer

**Name:** Chidi
**Role:** Backend engineer building a new microservice
**Goals:** Register his service with the gateway and have auth, rate limiting, and logging handled without writing any middleware.
**Pain Point:** Currently copies JWT middleware from another service and has to maintain it separately when the signing key rotates.
**Interaction with GoGate:** Adds a single entry to `config.yaml`, sets `skip_auth: false`, and his service immediately receives `X-User-ID`, `X-Tenant-ID`, and `X-Request-ID` headers on every authenticated request.

### 5.2 Platform / DevOps Engineer

**Name:** Amaka
**Role:** Infrastructure engineer responsible for reliability and observability
**Goals:** Single Prometheus scrape target, Grafana dashboard, and alerting for the entire API surface.
**Pain Point:** Currently has to configure metrics scraping for each individual service separately. Service-level logs are stored in different formats.
**Interaction with GoGate:** Scrapes `/metrics` once. Service-level request counts, latencies, and error rates appear in one dashboard; tenant-level rate limit views are optionally enabled when needed.

### 5.3 Security Engineer

**Name:** Tunde
**Role:** Reviews and enforces authentication and access control policies
**Goals:** Know that JWT validation is consistent across all APIs, rate limits cannot be bypassed, and CORS headers are uniform.
**Pain Point:** Different teams implement JWT validation slightly differently, leading to inconsistent enforcement.
**Interaction with GoGate:** Reviews gateway config and auth middleware once. Changes to the JWT signing key are applied in one place with a gateway restart.

### 5.4 School SaaS Product Owner

**Name:** Ngozi
**Role:** Product owner for the multi-tenant school management SaaS
**Goals:** Each school's requests are strictly isolated. One school cannot accidentally see another's data at the network layer.
**Interaction with GoGate:** Tenant is resolved from the subdomain (`greenfield.schoolapp.com`), injected into context, and forwarded as a trusted `X-Tenant-ID` header to all downstream services. Row-level security in the database enforces the final isolation boundary.

---

## 6. System Architecture Overview

### 6.1 High-Level Topology

```
                         ┌─────────────────────────────────────────────┐
Internet / Clients       │              Load Balancer / Ingress          │
                         │         (TLS termination happens here)        │
                         └─────────────────────┬───────────────────────┘
                                               │ HTTP
                                               ▼
                         ┌─────────────────────────────────────────────┐
                         │                  G O G A T E                 │
                         │                                              │
                         │  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
                         │  │ Recovery │→ │RequestID │→ │  CORS    │  │
                         │  └──────────┘  └──────────┘  └──────────┘  │
                         │  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
                         │  │  Logger  │→ │ Tenant   │→ │ Metrics  │  │
                         │  └──────────┘  │ Resolver │  └──────────┘  │
                         │                └──────────┘                 │
                         │         ┌──────────────────────┐            │
                         │         │     Route Match      │            │
                         │         └───────────┬──────────┘            │
                         │  ┌──────────────────▼──────────────────┐   │
                         │  │  Per-Route Middleware Chain          │   │
                         │  │  [RateLimiter] → [JWTVerifier]      │   │
                         │  └──────────────────┬──────────────────┘   │
                         │                     │                       │
                         └─────────────────────┼───────────────────────┘
                                               │
                    ┌──────────────────────────┼──────────────────────────┐
                    │                          │                          │
                    ▼                          ▼                          ▼
          ┌─────────────────┐       ┌──────────────────┐       ┌──────────────────┐
          │  Auth Service   │       │  School Service  │       │ Billing Service  │
          │  :8081          │       │  :8082           │       │  :8083           │
          └─────────────────┘       └──────────────────┘       └──────────────────┘

                    ┌──────────────────────────┐
                    │         Redis            │
                    │  (rate limiting cache)   │
                    └──────────────────────────┘
```

### 6.2 Request Lifecycle

Every HTTP request passes through the following pipeline in order:

1. **Recovery** — Defers a panic handler. If any downstream code panics, returns HTTP 500 without crashing the process.
2. **RequestID** — Checks `X-Request-ID` header. If absent, generates a UUID4. Injects into context and response headers.
3. **RealIP** — Extracts client IP from `X-Forwarded-For` / `X-Real-IP` only when the immediate peer is in a configured trusted proxy CIDR list. Otherwise uses the socket remote address.
4. **CORS** — Evaluates preflight requests and sets `Access-Control-*` headers per the configured allowed origins list.
5. **Logger** — Emits a structured JSON log entry at request start with method, path, tenant ID, and request ID.
6. **Route Match** — chi router matches the request path prefix to a registered service.
7. **TenantResolver** *(conditional)* — If matched service is `tenant_aware: true`, extracts tenant identity using the configured strategy, injects tenant ID into context, and returns HTTP 400 if unresolved.
8. **Metrics** — Records Prometheus observations (increments request counter, starts latency histogram timer).
9. **RateLimiter** — Checks Redis for the current request count in the sliding window for this tenant + service. Returns HTTP 429 with `X-RateLimit-*` headers if limit exceeded.
10. **JWTVerifier** *(conditional)* — Parses and validates the Bearer token. Injects `X-User-ID` and `X-User-Roles` headers. For tenant-aware routes, verifies JWT `tenant_id` matches resolved tenant and returns HTTP 403 on mismatch.
11. **ReverseProxy** — Forwards the modified request to the upstream service target. Streams the response back to the client.
12. **Logger** *(completion)* — Emits log entry at request completion with status code and latency.
13. **Metrics** *(completion)* — Records final Prometheus histogram observation.

---

## 7. Functional Requirements

### 7.1 Routing

| ID | Requirement | Priority |
|---|---|---|
| FR-R01 | The gateway MUST route requests to upstream services based on URL path prefix matching. | Must Have |
| FR-R02 | Route configuration MUST be loaded from a YAML file at startup. | Must Have |
| FR-R03 | Adding a new upstream service MUST require only a YAML config change and gateway restart — no code changes. | Must Have |
| FR-R04 | The gateway MUST support any number of registered services (no hardcoded service list). | Must Have |
| FR-R05 | If no route matches the request path, the gateway MUST return HTTP 404 with a structured JSON error body. | Must Have |
| FR-R06 | The gateway MUST strip the matched prefix before forwarding to the upstream or preserve it, based on a config flag. | Should Have |
| FR-R07 | The gateway MUST support path-based versioning (e.g. `/api/v1/`, `/api/v2/`) as separate route prefixes. | Should Have |

### 7.2 Authentication

| ID | Requirement | Priority |
|---|---|---|
| FR-A01 | The gateway MUST validate JWT tokens on all routes where `skip_auth: false`. | Must Have |
| FR-A02 | JWT validation MUST verify the token signature, expiry (`exp`), and issuer (`iss`). | Must Have |
| FR-A03 | On successful validation, the gateway MUST inject `X-User-ID` and `X-User-Roles` headers before forwarding. | Must Have |
| FR-A04 | On validation failure, the gateway MUST return HTTP 401 with a structured error body and MUST NOT forward the request. | Must Have |
| FR-A05 | Routes with `skip_auth: true` MUST bypass JWT validation entirely. | Must Have |
| FR-A06 | The gateway MUST support Bearer tokens passed in the `Authorization` header. | Must Have |
| FR-A07 | The gateway MUST support Bearer tokens passed as a `token` query parameter for WebSocket upgrade requests. | Should Have |
| FR-A08 | Signing keys MUST be configurable via environment variables or remote JWKS and MUST NOT be committed to config in production. | Must Have |
| FR-A09 | The gateway MUST support key rotation without downtime by accepting multiple active keys and resolving by `kid`. | Should Have |
| FR-A10 | For tenant-aware routes, the gateway MUST compare resolved tenant ID with JWT `tenant_id` claim and reject mismatches with HTTP 403. | Must Have |
| FR-A11 | The gateway SHOULD support JWKS retrieval and cache key material with configurable TTL and refresh timeout. | Should Have |

### 7.3 Tenant Resolution

| ID | Requirement | Priority |
|---|---|---|
| FR-T01 | The gateway MUST support three tenant resolution strategies: subdomain, header, and path prefix. | Must Have |
| FR-T02 | The active strategy MUST be configurable per-deployment via the config file. | Must Have |
| FR-T03 | Per-service tenant awareness MUST be configurable (`tenant_aware: true/false`). Services with `tenant_aware: false` MUST skip tenant resolution. | Must Have |
| FR-T04 | The resolved tenant ID MUST be injected into the request context and forwarded as the `X-Tenant-ID` header to downstream services. | Must Have |
| FR-T05 | If `tenant_aware: true` and tenant cannot be resolved, the gateway MUST return HTTP 400 with a descriptive error. | Must Have |
| FR-T06 | The subdomain strategy MUST ignore reserved subdomains (`www`, `api`, `admin`) and treat them as unresolvable tenants. | Must Have |
| FR-T07 | Tenant IDs MUST be validated against a configurable allowlist pattern (e.g. alphanumeric + hyphens only) to prevent injection attacks. | Should Have |

### 7.4 Rate Limiting

| ID | Requirement | Priority |
|---|---|---|
| FR-RL01 | The gateway MUST enforce request rate limits per tenant per service. | Must Have |
| FR-RL02 | Rate limiting MUST use a sliding window algorithm (not fixed window) to prevent burst spikes at window boundaries. | Must Have |
| FR-RL03 | Rate limit counters MUST be stored in Redis to ensure consistency across multiple gateway instances. | Must Have |
| FR-RL04 | Each service MUST have a configurable `rate_limit_rpm` (requests per minute). If not set, the global default applies. | Must Have |
| FR-RL05 | When a rate limit is exceeded, the gateway MUST return HTTP 429 with the following headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`. | Must Have |
| FR-RL06 | For non-tenant-aware routes, rate limiting MUST fall back to per-IP limiting. | Must Have |
| FR-RL07 | If Redis is unavailable and `rate_limit.fail_open: true`, the gateway MUST allow requests through and log a warning rather than rejecting all traffic. | Must Have |
| FR-RL08 | Rate limit configuration MUST support a burst allowance on top of the base RPM limit. | Should Have |
| FR-RL09 | The gateway MUST support per-plan rate limits (e.g. free tier: 100 RPM, pro tier: 5000 RPM) driven by a claim in the JWT. | Could Have |

### 7.5 Reverse Proxy

| ID | Requirement | Priority |
|---|---|---|
| FR-P01 | The gateway MUST proxy all matched requests to the configured upstream target using `net/http/httputil.ReverseProxy`. | Must Have |
| FR-P02 | The proxy MUST support connection pooling to upstream services (configurable `MaxIdleConns`, `MaxIdleConnsPerHost`). | Must Have |
| FR-P03 | The proxy MUST set configurable read, write, and idle timeouts on upstream connections. | Must Have |
| FR-P04 | If the upstream service is unreachable or times out, the gateway MUST return HTTP 503/504 with a structured error body. Upstream HTTP status codes (including 4xx/5xx) MUST be passed through unchanged. | Must Have |
| FR-P05 | The gateway MUST forward the `X-Request-ID` header to every upstream request to enable distributed tracing. | Must Have |
| FR-P06 | The gateway MUST strip sensitive headers (`Cookie`, original `Authorization`) before forwarding unless explicitly configured to forward them. | Should Have |
| FR-P07 | The gateway MUST support response streaming (not buffer entire response in memory). | Must Have |
| FR-P08 | The gateway MUST support WebSocket upgrade proxying. | Should Have |
| FR-P09 | The gateway MUST strip client-supplied trusted identity headers (`X-User-ID`, `X-Tenant-ID`, `X-User-Roles`) and re-inject canonical values only after middleware evaluation. | Must Have |

### 7.6 Middleware

| ID | Requirement | Priority |
|---|---|---|
| FR-M01 | The gateway MUST inject a unique `X-Request-ID` into every request that does not already carry one. | Must Have |
| FR-M02 | The gateway MUST recover from panics in any part of the request pipeline and return HTTP 500 without crashing. | Must Have |
| FR-M03 | The gateway MUST handle CORS preflight (`OPTIONS`) requests and set appropriate `Access-Control-*` response headers. | Must Have |
| FR-M04 | CORS allowed origins MUST be configurable per deployment. | Must Have |
| FR-M05 | The gateway MUST expose `GET /health` returning HTTP 200 with `{"status":"ok"}` for liveness probes. | Must Have |
| FR-M06 | The gateway MUST expose `GET /ready` returning HTTP 200 when config is loaded and the gateway can serve traffic. Redis disconnection MUST return a degraded readiness payload when `rate_limit.fail_open: true`, and HTTP 503 when `rate_limit.fail_open: false`. | Must Have |
| FR-M07 | The gateway MUST accept and trust forwarding headers (`X-Forwarded-For`, `X-Real-IP`) only from configured trusted proxies. | Must Have |

### 7.7 Configuration

| ID | Requirement | Priority |
|---|---|---|
| FR-C01 | All gateway configuration MUST be loadable from a single YAML file. | Must Have |
| FR-C02 | Sensitive config values (JWT keys, Redis password) MUST be overridable via environment variables. | Must Have |
| FR-C03 | The gateway MUST log a clear error and exit on startup if required configuration is missing or malformed. | Must Have |
| FR-C04 | The config file path MUST be specifiable as a CLI flag or environment variable. | Should Have |
| FR-C05 | The gateway SHOULD support hot-reload of non-sensitive configuration (service list, rate limits) without a full restart. | Could Have |

---

## 8. Non-Functional Requirements

### 8.1 Performance

| ID | Requirement | Target |
|---|---|---|
| NFR-P01 | Gateway-added latency overhead (p50) | < 5ms |
| NFR-P02 | Gateway-added latency overhead (p99) | < 20ms |
| NFR-P03 | Throughput under sustained load | > 10,000 RPS per instance |
| NFR-P04 | Memory footprint at idle | < 50MB RSS |
| NFR-P05 | Memory footprint under 10,000 RPS | < 256MB RSS |
| NFR-P06 | Goroutine leak policy | Zero goroutine leaks over 24-hour soak test |
| NFR-P07 | Redis operation latency (p99) | < 2ms (local Redis) |

### 8.2 Availability

| ID | Requirement | Target |
|---|---|---|
| NFR-A01 | Uptime SLA | 99.95% (< 22 min downtime/month) |
| NFR-A02 | Graceful shutdown drain timeout | 30 seconds |
| NFR-A03 | Restart time (cold start) | < 2 seconds |
| NFR-A04 | Behaviour during Redis downtime | Fail open (rate limiting disabled, gateway continues routing) |
| NFR-A05 | Minimum replicas in Kubernetes | 2 (HPA minimum) |
| NFR-A06 | Maximum replicas in Kubernetes | 20 (HPA maximum) |
| NFR-A07 | Readiness behavior during Redis downtime (fail-open mode) | `/ready` remains 200 with `status=degraded` |

### 8.3 Scalability

| ID | Requirement | Target |
|---|---|---|
| NFR-S01 | Horizontal scaling | Stateless — any number of instances can run behind a load balancer |
| NFR-S02 | Number of registered services | No technical upper limit; tested up to 100 services |
| NFR-S03 | Number of concurrent tenants | Tested up to 10,000 active tenants |
| NFR-S04 | Rate limit state | Centralised in Redis; consistent across all gateway instances |

### 8.4 Security

| ID | Requirement |
|---|---|
| NFR-SEC01 | JWT keys/secrets MUST never appear in logs, stack traces, or error responses. |
| NFR-SEC02 | All HTTP responses MUST include `X-Content-Type-Options: nosniff` and `X-Frame-Options: DENY`. |
| NFR-SEC03 | The gateway MUST NOT expose internal error details (stack traces, file paths) in HTTP error responses. |
| NFR-SEC04 | Redis connection MUST support password authentication and optionally TLS. |
| NFR-SEC05 | The Docker image MUST run as a non-root user. |
| NFR-SEC06 | The production Docker image MUST be built from a `scratch` base (no shell, no package manager). |

### 8.5 Maintainability

| ID | Requirement |
|---|---|
| NFR-M01 | Test coverage MUST be ≥ 80% on all `internal/` packages. |
| NFR-M02 | All public functions and types MUST have Go doc comments. |
| NFR-M03 | No direct external dependency on any commercial product or paid service. |
| NFR-M04 | The binary MUST be statically compiled (`CGO_ENABLED=0`). |
| NFR-M05 | All linting rules defined in `.golangci.yml` MUST pass in CI. |
| NFR-M06 | OSS release artifacts (`LICENSE`, `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`) MUST be present before public release. |

---

## 9. Technical Specifications

### 9.1 Language and Runtime

| Component | Choice | Rationale |
|---|---|---|
| Language | Go 1.22+ | Static binary, low GC pause, excellent concurrency primitives, `net/http` stdlib is production-grade |
| HTTP Router | `go-chi/chi` v5 | Lightweight, stdlib-compatible, supports middleware chains per route group |
| Reverse Proxy | `net/http/httputil` | Stdlib — no external dependency, full control, supports streaming |
| JWT | `golang-jwt/jwt` v5 | Actively maintained, supports HS256/RS256/ES256 |
| Redis Client | `redis/go-redis` v9 | Context-aware, supports pipelining for atomic rate limit operations |
| Config | `spf13/viper` | YAML + env override, widely used in Go ecosystem |
| Logging | `rs/zerolog` | Zero-allocation structured JSON logger, lowest overhead in Go |
| Metrics | `prometheus/client_golang` | Industry standard; integrates with every observability stack |
| UUID | `google/uuid` | RFC 4122 compliant, used for request ID generation |

### 9.2 Project Layout

The project follows the [Standard Go Project Layout](https://github.com/golang-standards/project-layout):

```
api-gateway/
├── cmd/
│   └── gateway/
│       └── main.go                   # Binary entrypoint
├── internal/                         # Private application packages
│   ├── auth/
│   │   ├── jwt.go                    # JWT verification middleware
│   │   └── jwt_test.go
│   ├── config/
│   │   └── config.go                 # Viper config loader and structs
│   ├── logger/
│   │   └── logger.go                 # Zerolog initialisation
│   ├── metrics/
│   │   └── metrics.go                # Prometheus counter/histogram definitions
│   ├── middleware/
│   │   ├── common.go                 # RequestID, Logger, Recovery
│   │   └── tenant.go                 # Tenant resolver middleware wrapper
│   ├── proxy/
│   │   └── proxy.go                  # Reverse proxy with error handling
│   ├── ratelimit/
│   │   └── limiter.go                # Redis sliding window rate limiter
│   └── tenant/
│       ├── resolver.go               # Subdomain / header / path strategies
│       └── resolver_test.go
├── pkg/                              # Shared, importable packages
│   ├── contextkeys/
│   │   └── keys.go                   # Typed context key constants
│   └── response/
│       └── response.go               # Standard JSON response helpers
├── tests/
│   └── integration_test.go           # Integration tests against mock backends
├── scripts/
│   └── gen_token.go                  # Dev utility to generate test JWTs
├── deployments/
│   ├── docker/
│   │   ├── Dockerfile                # Multi-stage build → scratch image
│   │   ├── docker-compose.yml        # Full local dev stack
│   │   └── prometheus.yml            # Prometheus scrape config
│   └── k8s/
│       └── gateway.yaml              # Deployment, Service, Ingress, HPA
├── .github/
│   └── workflows/
│       └── ci.yml                    # GitHub Actions: test, lint, build, push
├── config.yaml                       # Default configuration file
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### 9.3 Dependency Management

- Dependencies are managed via Go modules (`go.mod`, `go.sum`).
- `go mod tidy` is run as part of the CI pipeline to detect unused or missing dependencies.
- Dependabot or Renovate Bot is configured to open PRs for dependency updates weekly.
- No `replace` directives are used in `go.mod` in production builds.

### 9.4 Build System

```makefile
# Core targets required
run          # go run ./cmd/gateway
build        # produce bin/gateway static binary
test         # go test ./... -race -cover
lint         # golangci-lint run
docker-up    # docker compose up --build -d
docker-down  # docker compose down
gen-token    # generate test JWT for local development
```

---

## 10. Data Models

### 10.1 Configuration Schema

```yaml
# Full annotated config.yaml schema

server:
  port: 8080                     # int — TCP port to listen on
  read_timeout: 30s              # duration — max time to read request headers + body
  write_timeout: 30s             # duration — max time to write response
  idle_timeout: 120s             # duration — max time to keep idle connections open
  trusted_proxies:               # list[string] — CIDR allowlist for trusting X-Forwarded-* headers
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"

redis:
  addr: "localhost:6379"         # string — Redis host:port
  password: ""                   # string — Redis AUTH password (prefer env var)
  db: 0                          # int — Redis database index

jwt:
  issuer: "api-gateway"          # string — expected iss claim value
  algorithms: ["HS256"]          # array — accepted signing algorithms
  clock_skew: 30s                # duration — allowable skew for exp/nbf checks
  keys:                          # list — local keys for verification and rotation
    - kid: "primary-hs256"
      kty: "oct"
      value: ""                  # string — set via JWT_KEY_PRIMARY env
      primary: true
  jwks_url: ""                   # string — optional remote JWKS endpoint
  jwks_cache_ttl: 5m             # duration — JWKS cache duration

rate_limit:
  default_rpm: 1000              # int — default requests per minute per tenant
  burst: 50                      # int — extra requests allowed above RPM briefly
  fail_open: true                # bool — if true, continue serving traffic when Redis is unavailable

tenant:
  strategy: "subdomain"          # enum: subdomain | header | path
  header_name: "X-Tenant-ID"    # string — header name when strategy = header

cors:
  allowed_origins: []            # list[string] — origins permitted to make cross-origin requests; empty disables CORS; ["*"] allows all
  allowed_methods:               # list[string] — HTTP methods allowed in preflight (defaults: GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD)
    - "GET"
    - "POST"
  allowed_headers:               # list[string] — headers the client may send (defaults: Authorization, Content-Type, X-Request-ID, X-Tenant-ID)
    - "Authorization"
    - "Content-Type"
  exposed_headers:               # list[string] — headers the browser may read from the response (default: X-Request-ID)
    - "X-Request-ID"
  allow_credentials: false       # bool — whether to send Access-Control-Allow-Credentials: true
  max_age: 86400                 # int — preflight cache duration in seconds (default: 86400)

services:
  - name: ""                     # string — human-readable service name (used in logs/metrics)
    prefix: ""                   # string — URL path prefix to match (e.g. /api/v1/auth)
    target: ""                   # string — upstream URL (e.g. http://auth-service:8081)
    skip_auth: false             # bool — bypass JWT validation for this service
    tenant_aware: false          # bool — resolve and inject tenant for this service
    rate_limit_rpm: 0            # int — service-specific RPM (0 = use global default)

logging:
  level: "info"                  # enum: debug | info | warn | error
  format: "json"                 # enum: json | pretty

metrics:
  enabled: true                  # bool — expose /metrics endpoint
  path: "/metrics"               # string — path to expose metrics on
  include_tenant_metrics: false  # bool — high-cardinality tenant metrics (off by default)
```

### 10.2 JWT Claims Schema

The gateway expects JWT tokens to carry the following claims:

```json
{
  "user_id":   "string (required) — internal user identifier",
  "tenant_id": "string (optional) — tenant the user belongs to; only validated when the matched route has tenant_aware: true (see FR-A10)",
  "roles":     ["string (optional) — array of role strings"],
  "iss":       "string (required) — must match jwt.issuer in config",
  "sub":       "string (optional) — subject (usually same as user_id)",
  "iat":       "integer (recommended) — issued at (Unix timestamp)",
  "exp":       "integer (required) — expiry (Unix timestamp)"
}
```

The following standard claims are validated: `exp` (must not be expired), `iss` (must match configured issuer). When multiple keys are configured, `kid` is used to select the verification key. The `roles` claim is forwarded as a comma-separated string in `X-User-Roles`.

**`tenant_id` validation rules:** The `tenant_id` claim is optional in the token. It is only enforced when the matched service route has `tenant_aware: true` configured. In that case, the gateway resolves the tenant from the request (via subdomain, header, or path strategy) and compares it against the JWT `tenant_id` claim — a mismatch returns HTTP 403. For non-tenant-aware routes, `tenant_id` is forwarded as `X-Tenant-ID` if present but not validated.

### 10.3 Rate Limit Redis Key Schema

```
Key:   rate:<tenant_id>:<path_prefix_20_chars>
Type:  Sorted Set (ZSET)
Score: Unix millisecond timestamp of each request
TTL:   2 minutes (auto-expiry for cleanup)

Example:
  rate:greenfield:/api/v1/schools
  rate:192.168.1.1:/api/v1/auth    (IP-based for non-tenant routes)
```

### 10.4 Standard Error Response Body

All error responses returned by the gateway use this schema:

```json
{
  "success": false,
  "error": "human-readable error message"
}
```

All success pass-throughs from upstream services are streamed directly and do not wrap the body. Only gateway-originated error responses use this schema.

### 10.5 Standard Log Entry Schema

Every request log entry emitted by the gateway contains the following fields:

```json
{
  "level":       "info",
  "time":        "2026-03-20T12:34:56Z",
  "request_id":  "550e8400-e29b-41d4-a716-446655440000",
  "tenant_id":   "greenfield",
  "method":      "GET",
  "path":        "/api/v1/schools/list",
  "remote_addr": "41.58.12.34",
  "user_agent":  "Mozilla/5.0 ...",
  "status":      200,
  "latency_ms":  4,
  "service":     "school-service",
  "message":     "request completed"
}
```

---

## 11. API Specification

### 11.1 System Endpoints (Gateway-Native)

These endpoints are served directly by the gateway and do not proxy to any upstream service.

#### GET /health

Returns gateway liveness status. Used by Kubernetes liveness probes.

**Response: 200 OK**
```json
{ "status": "ok", "service": "api-gateway" }
```

#### GET /ready

Returns gateway readiness. Returns 200 when config is loaded and the gateway can serve traffic. Used by Kubernetes readiness probes.

**Response: 200 OK**
```json
{ "status": "ok", "service": "api-gateway", "redis": "connected" }
```

**Response: 200 OK (degraded, fail-open mode)** (Redis not reachable, `rate_limit.fail_open=true`)
```json
{ "status": "degraded", "service": "api-gateway", "redis": "disconnected", "rate_limit": "disabled" }
```

**Response: 503 Service Unavailable** (critical dependency unavailable in fail-closed mode)
```json
{ "status": "unavailable", "service": "api-gateway", "redis": "disconnected", "rate_limit": "required" }
```

#### GET /metrics

Exposes Prometheus-formatted metrics. Should be accessible only from within the cluster (not exposed publicly).

**Response: 200 OK** — Prometheus text format

### 11.2 Proxied Route Convention

All proxied routes follow this convention:

```
{tenant_subdomain}.{base_domain}/{service_prefix}/{resource_path}

Example:
  greenfield.schoolapp.com/api/v1/schools/students?grade=10

  Resolves to:
    tenant_id:    greenfield
    upstream:     http://school-service:8082
    forwarded as: http://school-service:8082/api/v1/schools/students?grade=10
```

### 11.3 Headers — Inbound (Client to Gateway)

| Header | Required | Description |
|---|---|---|
| `Authorization` | Conditional | `Bearer <jwt_token>` — required on auth-protected routes |
| `X-Request-ID` | Optional | If provided, used as-is. If absent, gateway generates a UUID4. |
| `X-Tenant-ID` | Conditional | Required when tenant strategy is `header` |
| `Content-Type` | Optional | Forwarded as-is to upstream |
| `X-User-ID`, `X-User-Roles` | Ignored | If sent by client, stripped and replaced by gateway-derived values |

### 11.4 Headers — Outbound (Gateway to Upstream)

| Header | Source | Description |
|---|---|---|
| `X-User-ID` | JWT `user_id` claim | Authenticated user ID |
| `X-Tenant-ID` | Resolved tenant | Tenant identifier |
| `X-User-Roles` | JWT `roles` claim | Comma-separated role list |
| `X-Request-ID` | Generated or forwarded | Unique request trace ID |
| `X-Gateway-Version` | Static | Gateway version string |
| `X-Forwarded-Host` | Original `Host` header | Original host before proxy |
| `X-Forwarded-For` | Set by proxy | Client IP address derived from trusted proxy chain |

### 11.5 Response Headers (Gateway to Client)

| Header | Value | Description |
|---|---|---|
| `X-Request-ID` | UUID4 | Trace ID for this request |
| `X-RateLimit-Limit` | Integer | Configured RPM limit |
| `X-RateLimit-Remaining` | Integer | Remaining requests in current window |
| `X-RateLimit-Reset` | Unix timestamp | When the current window resets |
| `X-Content-Type-Options` | `nosniff` | Security header |
| `X-Frame-Options` | `DENY` | Security header |

---

## 12. Security Requirements

### 12.1 Authentication Security

- JWT tokens MUST be signed with HMAC-SHA256 (HS256) minimum. RS256 and ES256 MUST also be supported for environments using asymmetric signing keys.
- The gateway MUST reject tokens with the `none` algorithm.
- The gateway MUST validate the `exp` claim and reject expired tokens with HTTP 401.
- The gateway MUST validate the `iss` claim and reject tokens from unexpected issuers.
- Local symmetric keys MUST be at least 32 bytes in length. The gateway SHOULD log a warning if a shorter key is configured.
- When multiple signing keys are active, tokens MUST carry `kid` and the gateway MUST resolve verification keys by `kid`.
- For tenant-aware routes, JWT `tenant_id` MUST match the resolved tenant or the request is rejected with HTTP 403.
- The gateway MUST NOT log the JWT token string at any log level.

### 12.2 Transport Security

- TLS termination is handled upstream (load balancer / Ingress). The gateway runs plaintext HTTP internally.
- Redis connections MUST support TLS in production environments (configurable `redis.tls: true`).
- Internal service-to-service communication (gateway to upstream) is over private network only. mTLS is not required at the gateway layer in v1.0 but is a v2.0 consideration.

### 12.3 Header Security

The gateway MUST add the following security headers to every response:

```
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
```

In addition:

- The gateway MUST strip client-supplied `X-User-ID`, `X-Tenant-ID`, and `X-User-Roles` before proxying.
- The gateway MUST trust `X-Forwarded-For` / `X-Real-IP` only when the previous hop is in `server.trusted_proxies`.

### 12.4 CORS Policy

CORS configuration MUST be explicit — no wildcard (`*`) origin in production. Allowed origins MUST be specified as a whitelist in config:

```yaml
cors:
  allowed_origins:
    - "https://*.schoolapp.com"
    - "https://admin.schoolapp.com"
  allow_credentials: true
  max_age: 300
```

Wildcard subdomain origins MUST be implemented as explicit suffix matching (e.g. `*.schoolapp.com`), not as a literal `*` in `Access-Control-Allow-Origin`.

### 12.5 Rate Limit Security

Rate limits serve as a defense against denial-of-service attacks and credential stuffing. The auth service endpoint (`/api/v1/auth`) SHOULD have a stricter rate limit (e.g. 60 RPM) compared to general API endpoints.

### 12.6 Tenant Isolation Security

The gateway is the first enforcement point for tenant isolation at the network layer. However, tenant isolation MUST also be enforced at the database layer (Row-Level Security) and application layer. The gateway alone is not sufficient for complete tenant isolation.

Tenant IDs extracted from subdomains or headers MUST be validated against a safe character pattern (`^[a-z0-9][a-z0-9\-]{2,62}$`) before injection into context to prevent header injection attacks.

### 12.7 Secrets Management

In production, secrets MUST be provided via environment variables and injected from a secrets manager (AWS Secrets Manager, HashiCorp Vault, Kubernetes Secrets). They MUST NOT be committed to version control or baked into Docker images.

| Secret | Environment Variable | Description |
|---|---|---|
| JWT primary key | `JWT_KEY_PRIMARY` | Primary verification key (HMAC) |
| JWT secondary key(s) | `JWT_KEY_<KID>` | Additional active keys for rotation |
| JWT JWKS endpoint token (optional) | `JWT_JWKS_BEARER_TOKEN` | Bearer token for authenticated JWKS fetch |
| Redis password | `REDIS_PASSWORD` | Redis AUTH password |

---

## 13. Observability and Monitoring

### 13.1 Metrics

The following Prometheus metrics are exposed at `/metrics`:

| Metric Name | Type | Labels | Description |
|---|---|---|---|
| `gateway_http_requests_total` | Counter | `method`, `service`, `status`, `route` | Total requests by service and outcome |
| `gateway_http_request_duration_seconds` | Histogram | `method`, `service`, `route` | Request latency distribution |
| `gateway_active_connections` | Gauge | — | Number of currently active connections |
| `gateway_rate_limit_hits_total` | Counter | `service`, `mode` | Rate limit rejections by service and key mode (`tenant` or `ip`) |
| `gateway_tenant_rate_limit_hits_total` | Counter | `tenant_id` | Optional high-cardinality tenant signal, disabled by default |
| `gateway_upstream_errors_total` | Counter | `service` | Count of upstream proxy errors |

### 13.2 Alerting Rules (Recommended)

The following Prometheus alert rules are recommended for production:

```yaml
# Alert: High error rate
- alert: GatewayHighErrorRate
  expr: rate(gateway_http_requests_total{status=~"5.."}[5m]) > 0.05
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "Gateway error rate exceeds 5%"

# Alert: High p99 latency
- alert: GatewayHighLatency
  expr: histogram_quantile(0.99, rate(gateway_http_request_duration_seconds_bucket[5m])) > 0.5
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Gateway p99 latency exceeds 500ms"

# Alert: Redis unavailable
- alert: GatewayRedisDown
  expr: up{job="redis"} == 0
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "Redis is unreachable — rate limiting is disabled"

# Alert: Sustained rate limit hits for a tenant (requires metrics.include_tenant_metrics=true)
- alert: TenantRateLimitAbuse
  expr: rate(gateway_tenant_rate_limit_hits_total[5m]) > 10
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "A tenant is consistently hitting rate limits"
```

### 13.3 Logging

All logs are emitted as structured JSON to stdout. The following log levels are used:

| Level | When Used |
|---|---|
| `DEBUG` | Detailed request/response tracing, config loading. Disabled in production. |
| `INFO` | Every request completion (method, path, status, latency, tenant). Normal operation. |
| `WARN` | Rate limit hits, Redis unavailability, near-expired JWT tokens. |
| `ERROR` | Upstream proxy failures, panics, config errors, Redis pipeline failures. |

Log aggregation (Loki, CloudWatch, Datadog Logs) is performed externally. The gateway writes only to stdout.

### 13.4 Distributed Tracing

The `X-Request-ID` header is injected at the gateway and forwarded to all upstream services. This header MUST be included in all downstream service logs to enable request correlation across services.

In v2.0, OpenTelemetry trace context (`traceparent` header) propagation will be added to enable full distributed tracing with Jaeger or Tempo.

### 13.5 Grafana Dashboard

A Grafana dashboard JSON template is provided in `deployments/grafana/dashboard.json` (v2.0) with the following panels:

- Request rate by service (RPS)
- Error rate by service (%)
- p50 / p95 / p99 latency by service
- Active connections gauge
- Rate limit hits by service (default) and by tenant (optional high-cardinality view)
- Upstream error count by service

---

## 14. Deployment and Infrastructure

### 14.1 Docker

The production Docker image is built using a multi-stage build:

- **Stage 1 (builder):** `golang:1.22-alpine` — compiles the binary with `CGO_ENABLED=0 GOOS=linux`.
- **Stage 2 (runtime):** `scratch` — contains only the compiled binary, config file, and TLS CA certificates. No shell, no package manager, no OS utilities.

The final image is expected to be under 15MB.

```bash
docker build -f deployments/docker/Dockerfile -t gogate:latest .
docker run -p 8080:8080 \
  -e JWT_KEY_PRIMARY=your-production-key \
  -e REDIS_ADDR=redis.internal:6379 \
  -v ./config.yaml:/config.yaml:ro \
  gogate:latest
```

### 14.2 Kubernetes

The Kubernetes manifests in `deployments/k8s/gateway.yaml` define:

- **Deployment** — 3 replicas, rolling update strategy, resource requests and limits, liveness and readiness probes.
- **Service** — ClusterIP type, port 80 → 8080.
- **Ingress** — Wildcard subdomain routing (`*.yourapp.com`) with TLS via cert-manager.
- **HorizontalPodAutoscaler** — min 2, max 20 replicas, scale up at 70% CPU or 80% memory.

Resource limits per pod:

| Resource | Request | Limit |
|---|---|---|
| CPU | 100m | 500m |
| Memory | 64Mi | 256Mi |

### 14.3 Environment Stages

| Stage | Config Source | Redis | Replicas | TLS |
|---|---|---|---|---|
| Local Development | `config.yaml` | Docker Compose | 1 | None |
| Staging | `config.yaml` + env vars | Managed Redis | 1 | Let's Encrypt |
| Production | ConfigMap + Kubernetes Secrets | Managed Redis (HA) | 2–20 (HPA) | Let's Encrypt |

### 14.4 CI/CD Pipeline

The GitHub Actions pipeline (`/.github/workflows/ci.yml`) executes the following on every push:

1. **Checkout** — fetch code
2. **Setup Go** — install Go 1.22 with module cache
3. **Download dependencies** — `go mod download`
4. **Vet** — `go vet ./...`
5. **Test** — `go test ./... -race -coverprofile=coverage.out`
6. **Upload coverage** — to Codecov
7. **Build binary** — `CGO_ENABLED=0 GOOS=linux go build ...`
8. **Lint** — `golangci-lint run ./...`
9. **Security checks** — `gosec ./...` and `govulncheck ./...`
10. **Docker build + push** *(main branch only)* — push to GHCR with `latest` and SHA tags

### 14.5 Open-Source Release Readiness

The repository MUST include and maintain the following top-level documents before public release:

- `LICENSE` (AGPLv3)
- `LICENSE-COMMERCIAL.md` (commercial terms and contact)
- `README.md` (quick start, architecture, compatibility matrix)
- `CONTRIBUTING.md` (branching, tests, PR process, coding standards)
- `CLA.md` (contributor licensing and relicensing rights)
- `CODE_OF_CONDUCT.md` (contributor behavior policy)
- `SECURITY.md` (reporting channel, supported versions, disclosure timeline)
- `SUPPORT.md` (community support scope and response expectations)
- `TRADEMARKS.md` (name/logo usage and hosted-service branding boundaries)

Versioning policy:

- The project MUST follow SemVer.
- Breaking config or API behavior changes MUST increment major version.
- Every release MUST include migration notes and a changelog entry.

---

## 15. Testing Strategy

### 15.1 Test Pyramid

```
           ╱─────────────╲
          ╱  E2E Tests     ╲       ← Few (5–10 critical flows)
         ╱───────────────────╲
        ╱  Integration Tests  ╲    ← Moderate (1 per service interaction)
       ╱─────────────────────────╲
      ╱      Unit Tests           ╲ ← Many (every middleware, resolver, etc.)
     ╱─────────────────────────────╲
```

### 15.2 Unit Tests

Every package in `internal/` and `pkg/` MUST have unit tests. Key test cases:

**`internal/auth`**
- Valid token → next handler called, headers injected
- Expired token → 401 returned, next handler not called
- Wrong secret → 401 returned
- Missing Authorization header → 401 returned
- Malformed header (no Bearer prefix) → 401 returned
- Token with `none` algorithm → 401 returned
- Query param token for WebSocket → valid accepted

**`internal/tenant`**
- Subdomain: `greenfield.app.com` → `greenfield`
- Subdomain: `www.app.com` → error (reserved)
- Subdomain: `app.com` (no subdomain) → error
- Header strategy: header present → resolved
- Header strategy: header missing → error
- Path strategy: `/t/greenfield/...` → `greenfield`
- Path strategy: no prefix → error

**`internal/ratelimit`**
- Under limit → request passes, headers set
- Over limit → HTTP 429, headers set
- Redis failure → request passes (fail open)
- Burst allowance respected

**`pkg/response`**
- Each error helper returns correct status code
- Body is valid JSON
- `success: false` on all error responses

### 15.3 Integration Tests

Integration tests run against real Redis and mock HTTP backends (using `httptest.NewServer`).

Key integration test scenarios:

- Full request through gateway with valid JWT → upstream receives correct forwarded headers
- Full request with expired JWT → 401 returned before proxying
- Tenant mismatch (`tenant_id` claim != resolved tenant) → 403 returned before proxying
- Rate limit exceeded over multiple requests → 429 returned on correct request number
- Upstream service returns 500 → gateway passes through 500 unchanged
- Upstream timeout/unreachable → gateway returns 503/504 with structured error body
- Client attempts to spoof `X-User-ID` / `X-Tenant-ID` → spoofed values stripped; canonical values forwarded
- Unknown route → 404 with structured error body
- Health endpoint always returns 200 regardless of upstream status
- Ready endpoint returns 200 degraded when Redis is down and `fail_open=true`
- CORS preflight returns correct headers

### 15.4 Load Testing

Load tests are executed using [k6](https://k6.io/) against a staging environment before every major release.

Target scenarios:

| Scenario | VUs | Duration | Pass Criteria |
|---|---|---|---|
| Sustained baseline | 100 | 10 min | p99 < 20ms, error rate < 0.1% |
| Burst traffic | 1000 | 2 min | p99 < 50ms, no process crash |
| Rate limit enforcement | 200 | 5 min | All tenants correctly throttled at configured RPM |
| Redis failure simulation | 100 | 5 min | Gateway continues routing (fail open), no 5xx on non-RL requests |

### 15.5 Security Testing

- Static analysis with `gosec` run in CI on every push.
- Dependency vulnerability scanning with `govulncheck` run weekly.
- JWT edge cases (algorithm confusion, `none` algorithm) tested in unit tests.
- CORS origin bypass attempts tested in integration tests.
- Header spoofing attempts (`X-User-ID`, `X-Tenant-ID`, `X-Forwarded-For`) tested in integration tests with trusted and untrusted proxy scenarios.

---

## 16. Migration and Rollout Plan

### Phase 1 — Foundation (Weeks 1–2)

- Implement core reverse proxy with config-driven service registration.
- Implement structured logging and health endpoints.
- Set up Docker Compose local dev environment.
- Write unit tests for all core packages.
- Set up GitHub Actions CI pipeline.

**Acceptance criteria:** Gateway routes requests to mock upstream services. All unit tests pass with > 80% coverage. CI pipeline green.

### Phase 2 — Authentication and Tenancy (Weeks 3–4)

- Implement JWT verification middleware.
- Implement tenant resolver (all three strategies).
- Implement `tenant_aware` and `skip_auth` per-service config flags.
- Write unit and integration tests for auth and tenant packages.

**Acceptance criteria:** Protected routes reject unauthenticated requests. Tenant ID is correctly resolved and forwarded. Integration tests pass against mock backend.

### Phase 3 — Rate Limiting and Metrics (Weeks 5–6)

- Implement Redis-backed sliding window rate limiter.
- Implement Prometheus metrics middleware.
- Add Redis to Docker Compose stack.
- Write load tests for rate limiting correctness.

**Acceptance criteria:** Rate limits enforced correctly per tenant. Prometheus metrics endpoint exposes all defined metrics. Redis failure causes fail-open behaviour.

### Phase 4 — Production Hardening (Weeks 7–8)

- Implement graceful shutdown.
- Security header middleware.
- Multi-stage Dockerfile with scratch image.
- Kubernetes manifests (Deployment, Service, Ingress, HPA).
- Readiness probe supporting degraded fail-open mode when Redis is unavailable.
- Run load tests in staging environment.

**Acceptance criteria:** Docker image < 15MB. Kubernetes deployment healthy with HPA configured. Load test results meet NFR targets.

### Phase 5 — First Production Deployment

- Deploy to staging behind real Ingress controller.
- Validate wildcard TLS cert issuance via cert-manager.
- Run all integration and load tests against staging.
- Perform canary rollout to production (10% → 50% → 100% over 48 hours).
- Monitor Grafana dashboard for error rate, latency, and rate limit anomalies.

---

## 17. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Redis becomes a single point of failure for rate limiting | Medium | Medium | Fail-open behaviour (FR-RL07). Use Redis Sentinel or Redis Cluster in production. |
| JWT key rotation causes temporary auth failures | Medium | High | Support multiple active keys with `kid` lookup (FR-A09). Implement rolling rotation procedure. |
| Malformed config YAML causes silent misconfiguration | Low | High | Strict config validation on startup with clear error messages and non-zero exit code. |
| Memory leak from goroutines in reverse proxy | Low | High | Regular soak tests (24h) with pprof profiling. Goroutine count monitored in Prometheus. |
| Tenant ID injection attack via subdomain | Low | High | Strict allowlist pattern validation on resolved tenant IDs (FR-T07). |
| High-cardinality Prometheus labels cause metric explosion | Low | Medium | Default metrics avoid tenant labels. Tenant-level metrics are opt-in (`metrics.include_tenant_metrics`) and monitored with cardinality alerts. |
| Gateway becomes a bottleneck under traffic spikes | Medium | High | Horizontal scaling via HPA. Stateless design ensures linear scale-out. Load test before each major release. |
| Upstream service is slow and holds connections | Medium | Medium | Configurable write timeout on upstream connections (FR-P03). Monitor `gateway_active_connections`. |

---

## 18. Success Metrics and KPIs

### 18.1 Technical Health Metrics

| Metric | Target | Measurement |
|---|---|---|
| Gateway p50 latency overhead | < 5ms | Prometheus histogram, measured as gateway processing time minus upstream response time |
| Gateway p99 latency overhead | < 20ms | Prometheus histogram |
| Uptime | > 99.95% | External uptime monitoring |
| Test coverage | > 80% | Codecov report per PR |
| CI pipeline duration | < 4 minutes | GitHub Actions run time |
| Docker image size | < 15MB | Docker image layer inspection |

### 18.2 Operational Metrics

| Metric | Target | Measurement |
|---|---|---|
| Time to add a new service | < 5 minutes | YAML edit + gateway restart |
| Time to rotate JWT keys | < 10 minutes | Zero-downtime rotation procedure |
| Mean time to detect incident | < 2 minutes | Alert firing time from Prometheus |
| Mean time to resolve incident | < 30 minutes | Incident post-mortems |

### 18.3 Developer Experience Metrics

| Metric | Target |
|---|---|
| Time for new developer to run gateway locally | < 10 minutes (`make docker-up`) |
| Time to generate a test token | < 1 minute (`make gen-token`) |
| Onboarding a new project to the gateway | < 30 minutes (config change only) |

---

## 19. Future Roadmap

### v1.1 — Configuration Hot Reload
Allow service list and rate limit changes to be applied without a gateway restart by watching the config file for changes using `fsnotify`.

### v1.2 — Admin API
A read-only internal REST API for introspecting current routing table, active rate limit state per tenant, and gateway health. Available only on a separate admin port (e.g. 9090), not exposed publicly.

### v1.3 — Circuit Breaker
Per-service circuit breaker pattern (half-open → open → closed) to automatically stop forwarding traffic to consistently failing upstream services and return fast errors while the service recovers.

### v2.0 — OpenTelemetry Tracing
Propagate OpenTelemetry `traceparent` headers to enable full distributed tracing across all services. Export trace spans to Jaeger, Tempo, or a managed tracing service.

### v2.1 — mTLS Between Gateway and Upstreams
Support mutual TLS on upstream connections for environments requiring certificate-based service identity in addition to JWT-based user identity.

### v2.2 — Plugin System
A Go plugin interface allowing custom middleware to be registered without forking the gateway. Enables adding domain-specific request enrichment (e.g. injecting subscription plan tier from a lookup table) without modifying core code.

### v2.3 — Management UI
A web-based dashboard for viewing routing configuration, live traffic metrics, rate limit state, and tenant activity. Built with Next.js, served separately from the gateway binary.

### v3.0 — GraphQL Federation Gateway Mode
Optional mode to act as a GraphQL federated gateway, stitching together subgraphs from multiple services while still enforcing authentication and rate limiting.

---

## 20. Glossary

| Term | Definition |
|---|---|
| **API Gateway** | A server that acts as the single entry point for all client requests to a backend system, routing them to appropriate services. |
| **Tenant** | A distinct customer organisation using a multi-tenant SaaS product. Each tenant's data is isolated from all others. |
| **Multi-tenant** | An architecture where a single deployment of software serves multiple tenants, each with isolated data and configuration. |
| **JWT (JSON Web Token)** | An open standard (RFC 7519) for securely transmitting claims between parties as a signed JSON object. |
| **Claim** | A piece of information asserted about a subject, encoded within a JWT. |
| **Middleware** | A function that wraps an HTTP handler to execute logic before and/or after the handler processes a request. |
| **Reverse Proxy** | A server that accepts client requests and forwards them to upstream servers, returning the upstream response to the client. |
| **Rate Limiting** | The practice of restricting how many requests a client or tenant can make within a time window. |
| **Sliding Window** | A rate limiting algorithm that counts requests within a rolling time window rather than a fixed one, preventing burst spikes at window boundaries. |
| **Redis** | An in-memory data structure store used as a distributed cache for rate limit counters. |
| **Prometheus** | An open-source monitoring and alerting toolkit that collects metrics by scraping HTTP endpoints. |
| **HPA (Horizontal Pod Autoscaler)** | A Kubernetes controller that automatically scales the number of pod replicas based on observed CPU/memory usage. |
| **Graceful Shutdown** | A process shutdown strategy that allows in-flight requests to complete before the process exits. |
| **Fail Open** | A resilience strategy where a system allows requests through when a dependency (e.g. Redis) is unavailable, rather than rejecting all traffic. |
| **RPM** | Requests Per Minute — the unit used to configure rate limits in GoGate. |
| **CORS** | Cross-Origin Resource Sharing — a browser security policy controlling which origins can make requests to a given domain. |
| **p50 / p95 / p99** | Percentile latency metrics. p99 = 99% of requests complete within this duration. |

---

## 21. Appendix

### 21.1 Sequence Diagram — Authenticated Multi-Tenant Request

```
Client          Gateway         Redis           Upstream Service
  │                │               │                   │
  │─ GET /api/v1/schools/students ─▶│               │                   │
  │  Host: greenfield.app.com       │               │                   │
  │  Authorization: Bearer <jwt>    │               │                   │
  │                │               │               │                   │
  │          [Recovery wraps]       │               │                   │
  │          [RequestID injected]   │               │                   │
  │          [CORS evaluated]       │               │                   │
  │          [Logger: request in]   │               │                   │
  │          [Tenant resolved: "greenfield"] │                          │
  │          [Metrics: counter++]   │               │                   │
  │                │               │               │                   │
  │                │─ ZCARD rate:greenfield:/api/v ─▶│                   │
  │                │◀─ count: 42 ───────────────────│                   │
  │                │  (< 1000 RPM limit, allow)      │                   │
  │                │               │               │                   │
  │          [JWT validated]        │               │                   │
  │          [X-User-ID injected]   │               │                   │
  │          [X-Tenant-ID injected] │               │                   │
  │                │               │               │                   │
  │                │─ GET /api/v1/schools/students ─────────────────────▶│
  │                │  X-User-ID: user_123            │                   │
  │                │  X-Tenant-ID: greenfield        │                   │
  │                │  X-Request-ID: uuid             │                   │
  │                │               │               │                   │
  │                │◀────────────────────────────── 200 OK ─────────────│
  │                │               │               │                   │
  │          [Logger: request complete, 4ms]        │                   │
  │          [Metrics: histogram observed]          │                   │
  │                │               │               │                   │
  │◀─ 200 OK ──────│               │               │                   │
  │  X-Request-ID: uuid            │               │                   │
  │  X-RateLimit-Remaining: 957    │               │                   │
```

### 21.2 Sequence Diagram — Rate Limited Request

```
Client          Gateway         Redis
  │                │               │
  │─ POST /api/v1/auth/login ──────▶│               │
  │                │               │               │
  │                │─ ZCARD rate:203.0.113.1:/api/v ▶│
  │                │◀─ count: 301 ──────────────────│
  │                │  (> 300 RPM limit for auth)     │
  │                │               │               │
  │◀─ 429 Too Many Requests ───────│               │
  │  X-RateLimit-Limit: 300        │               │
  │  X-RateLimit-Remaining: 0      │               │
  │  X-RateLimit-Reset: 1742478000 │               │
  │  {"success":false,"error":"rate limit exceeded, please slow down"}
```

### 21.3 Config Reference — All Fields

See `config.yaml` in the repository root for the annotated full configuration reference. Every field documents its type, accepted values, default, and the environment variable override where applicable.

### 21.4 Related Documents

| Document | Location | Description |
|---|---|---|
| Architecture Decision Records | `/docs/adr/` | Recorded decisions on key technical choices |
| School SaaS Multi-Tenancy Design | `/docs/multitenancy.md` | Deep-dive into tenant isolation strategy |
| Runbook — JWT Key Rotation | `/docs/runbooks/jwt-rotation.md` | Step-by-step key rotation without downtime |
| Runbook — Redis Failover | `/docs/runbooks/redis-failover.md` | Procedure for Redis HA failover |
| Load Test Results | `/docs/load-tests/` | k6 benchmark results per release |

### 21.5 Revision History

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.2.0 | 2026-03-20 | Engineering Team | Updated licensing strategy to AGPLv3 + commercial dual-license model; added CLA and trademark policy requirements |
| 1.1.0 | 2026-03-20 | Engineering Team | Resolved readiness/fail-open semantics, clarified upstream status behavior, added trusted-header and JWT key rotation model, reduced default metrics cardinality, added OSS release requirements |
| 1.0.0 | 2026-03-20 | Engineering Team | Initial draft |

---

*This document is maintained by the Engineering Team. For questions or corrections, open a GitHub issue in the `api-gateway` repository.*
