# GoGate v0.1.0 Roadmap

## 1. Purpose

Build a reusable, open-source API gateway that fronts all projects with a single Go binary and config-only onboarding, with a clear dual-licensing path for commercial use.  
Target release: **v0.1.0 on Friday, May 1, 2026**.

## 2. Scope

### In Scope for v0.1.0

- Config-driven path-prefix routing and reverse proxying.
- JWT verification with key rotation support (`kid`) and optional JWKS fetch.
- Tenant resolution (subdomain/header/path) with tenant claim enforcement.
- Redis-backed sliding-window rate limiting with fail-open toggle.
- Structured logs, Prometheus metrics, health/readiness endpoints.
- Trusted proxy handling and identity header stripping/reinjection.
- Docker image, local compose flow, Kubernetes deployment baseline.
- OSS release docs and contribution/security process.

### Out of Scope for v0.1.0

- Config hot reload.
- Admin UI/API.
- Circuit breaker.
- Plugin system.
- Full OpenTelemetry export pipeline.

## 3. Success Criteria

- Gateway overhead: p50 < 5ms, p99 < 20ms.
- Functional correctness: no auth bypass, no tenant isolation bypass.
- Redis outage behavior matches config (`fail_open=true/false`).
- New service onboarding via config change only.
- OSS/legal package complete (`LICENSE`, `LICENSE-COMMERCIAL.md`, `CONTRIBUTING.md`, `CLA.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, `SUPPORT.md`, `TRADEMARKS.md`).

## 4. Team and Owners

- **Product/PM**: scope, acceptance signoff, release decision.
- **Backend Lead**: gateway internals, middleware, proxy, auth, tenant, rate limit.
- **Platform/DevOps**: Docker/K8s, CI/CD, load test environment, release pipeline.
- **Security Lead**: auth/key handling, header trust model, vulnerability checks.
- **QA Lead**: integration/e2e test matrix, regression signoff.

## 5. Timeline and Milestones

| Week | Dates (2026) | Milestone | Exit Gate |
|---|---|---|---|
| Week 1 | Mar 23 - Mar 27 | Foundation | Proxy routes requests, strict config validation, CI baseline green |
| Week 2 | Mar 30 - Apr 3 | Auth + Tenant Enforcement | JWT validation, tenant resolver, tenant-claim mismatch enforcement |
| Week 3 | Apr 6 - Apr 10 | Rate Limiting + Metrics | Redis sliding window, fail-open/closed behavior, metrics baseline |
| Week 4 | Apr 13 - Apr 17 | Hardening | Timeouts, graceful shutdown, pass-through status behavior, websocket support |
| Week 5 | Apr 20 - Apr 24 | OSS Packaging | Public docs, examples, contribution/security policies, release flow |
| Week 6 | Apr 27 - May 1 | Pilot + Release | Canary success, runbooks validated, tagged `v0.1.0` |

## 6. Week-by-Week Delivery Plan

### Week 1: Foundation (Mar 23 - Mar 27)

- Build gateway bootstrap and config loading/validation.
- Implement route registry and reverse proxy path matching.
- Add request ID, recovery, structured request logging.
- Add `/health` and `/ready` endpoints.
- Stand up basic CI (`vet`, `test`, `lint`, build).

### Week 2: Authentication and Tenancy (Mar 30 - Apr 3)

- Add JWT middleware with keyset (`kid`) support.
- Add optional JWKS resolver + cache.
- Implement tenant resolution strategies and per-service `tenant_aware`.
- Enforce `tenant_id` claim vs resolved tenant match.
- Strip trusted identity headers before forwarding; re-inject canonical values.

### Week 3: Rate Limiting and Observability (Apr 6 - Apr 10)

- Implement Redis sliding-window limiter (Lua-backed).
- Add per-route and default RPM config.
- Add `fail_open` behavior and degraded readiness semantics.
- Publish Prometheus metrics with low-cardinality defaults.
- Add integration tests for rate limit and Redis outage behavior.

### Week 4: Production Hardening (Apr 13 - Apr 17)

- Configure upstream timeouts and connection pooling.
- Ensure upstream HTTP status passthrough and proper 503/504 mapping only for connectivity/timeouts.
- Add graceful shutdown request draining.
- Add WebSocket proxy handling and CORS hardening.
- Run soak + load tests and profile for leaks.

### Week 5: Open Source Packaging (Apr 20 - Apr 24)

- Add OSS policy docs and contribution guidelines.
- Finalize dual-licensing docs (AGPL community + commercial terms).
- Add example configs (single-tenant + multi-tenant).
- Finalize Dockerfile/compose and Kubernetes manifests.
- Expand CI with `gosec` and `govulncheck`.
- Produce migration and release notes template.

### Week 6: Pilot and Release (Apr 27 - May 1)

- Onboard one internal project through config only.
- Run staged canary (10% -> 50% -> 100%).
- Validate JWT key rotation and Redis failure runbooks.
- Fix release blockers and freeze scope.
- Tag and publish `v0.1.0`.

## 7. Release Gates

### Gate A: Foundation Complete (Mar 27)

- Route and proxy flow works for at least 2 mock upstream services.
- Startup rejects malformed config with clear errors.
- CI baseline green on default branch.

### Gate B: Security Path Complete (Apr 3)

- Invalid/expired JWT blocked.
- Tenant mismatch blocked with HTTP 403.
- Spoofed `X-User-ID`/`X-Tenant-ID` headers never forwarded.

### Gate C: Runtime Reliability Complete (Apr 10)

- Redis down + `fail_open=true`: requests pass, `/ready` degraded 200.
- Redis down + `fail_open=false`: `/ready` 503 and rate-limit path fails closed.
- Metrics endpoint stable and scrapeable.

### Gate D: Hardening Complete (Apr 17)

- No goroutine leaks in 24h soak.
- Upstream 4xx/5xx pass through unchanged.
- Timeout/unreachable mapped to 503/504 correctly.

### Gate E: OSS Ready (Apr 24)

- Required OSS/legal docs present and reviewed.
- Local quickstart works in under 10 minutes.
- External contributor path (fork -> test -> PR) is documented and tested.

### Gate F: Release Ready (May 1)

- Pilot project stable for 48h.
- No Sev-1/Sev-2 open defects.
- Release notes, changelog, and tag prepared.

## 8. Risks and Watch Items

- **Redis dependency risk**: mitigate with explicit fail-open/closed behavior and runbooks.
- **Security regression risk**: enforce header trust tests and tenant mismatch tests in CI.
- **Metrics cardinality drift**: keep tenant metrics opt-in and monitor series growth.
- **Scope creep risk**: defer roadmap items not required for `v0.1.0`.

## 9. Operating Cadence

- Daily 15-minute sync on blockers and gate status.
- Mid-week technical checkpoint (architecture and test progress).
- Friday gate review with explicit go/no-go decision.
- Release week daily canary review with rollback criteria.
