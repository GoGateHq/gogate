# GoGate v0.1.0 Execution Checklist

Use this file as the delivery board for the March 23, 2026 to May 1, 2026 timeline.

Owner roles:

- `BE` = Backend Lead
- `PLAT` = Platform/DevOps
- `SEC` = Security Lead
- `QA` = QA Lead
- `PM` = Product/PM

---

## Week 1 (Mar 23 - Mar 27): Foundation

| Done | ID | Task | Owner | Acceptance Test |
|---|---|---|---|---|
| [x] | W1-01 | Create gateway entrypoint and config bootstrap | BE | `go run ./cmd/gateway` starts with valid config and exits non-zero on invalid config |
| [x] | W1-02 | Implement strict config validation (required fields, bad values) | BE | Unit tests cover missing server port, invalid target URL, duplicate prefixes |
| [x] | W1-03 | Implement route registry and prefix-based reverse proxy | BE | Integration test routes 2 services by prefix and returns structured 404 on unknown route |
| [x] | W1-04 | Add recovery middleware + request ID injection | BE | Panic test returns 500 without process crash; `X-Request-ID` always present |
| [x] | W1-05 | Add JSON request completion logs | BE | Log line includes method, path, status, latency, request ID |
| [x] | W1-06 | Add `/health` and base `/ready` handlers | BE | `/health` returns 200 always; `/ready` returns 200 when boot is complete |
| [x] | W1-07 | Set up CI baseline (`vet`, `test`, `lint`, build) | PLAT | CI pipeline green on default branch |
| [ ] | W1-08 | Define Gate A signoff and close week | PM + QA | Gate A checklist signed with no Sev-1/Sev-2 defects |

---

## Week 2 (Mar 30 - Apr 3): Authentication and Tenancy

W2 completion rule: Checkboxes indicate Gate A approval (W1-08). Tests should be recorded in "Test Status" when run; "Gate Approved" tracks whether W1-08 has been signed off.

| Done | ID | Task | Owner | Acceptance Test | Test Status | Gate Approved |
|---|---|---|---|---|---|---|
| [ ] | W2-01 | Implement JWT verification middleware with supported alg list | BE + SEC | Valid token passes; expired/invalid token returns 401 | PASS (local tests) | Pending W1-08 |
| [ ] | W2-02 | Implement keyset-based validation (`kid`) | BE + SEC | Tokens signed by old and new active keys both validate during rotation window | PASS (local tests) | Pending W1-08 |
| [ ] | W2-03 | Implement optional JWKS fetch + cache TTL | BE | JWKS-backed token validates; stale cache refreshes on TTL expiry | PASS (local tests) | Pending W1-08 |
| [ ] | W2-04 | Implement tenant resolvers (subdomain/header/path) | BE | Unit tests cover valid and invalid cases for all 3 strategies | PASS (local tests) | Pending W1-08 |
| [ ] | W2-05 | Enforce tenant match (`resolved tenant == JWT tenant_id`) | BE + SEC | Mismatch returns 403 before proxying | PASS (local tests) | Pending W1-08 |
| [ ] | W2-06 | Strip spoofed identity headers and re-inject canonical headers | BE + SEC | Client `X-User-ID`/`X-Tenant-ID` values never reach upstream unchanged | PASS (local tests) | Pending W1-08 |
| [ ] | W2-07 | Add trusted proxy handling for `X-Forwarded-For`/`X-Real-IP` | BE + SEC | Untrusted hop cannot spoof client IP; trusted hop can forward expected value | PASS (local tests) | Pending W1-08 |
| [ ] | W2-08 | Gate B signoff | PM + QA + SEC | Auth and tenant enforcement tests pass in CI and staging | | |

---

## Week 3 (Apr 6 - Apr 10): Rate Limiting and Metrics

| Done | ID | Task | Owner | Acceptance Test |
|---|---|---|---|---|
| [ ] | W3-01 | Implement Redis sliding-window limiter (Lua script) | BE | Over-limit request returns 429 with `X-RateLimit-*` headers |
| [ ] | W3-02 | Add per-service `rate_limit_rpm` with global fallback | BE | Service override applies correctly and defaults work when unset |
| [ ] | W3-03 | Implement `rate_limit.fail_open` behavior | BE | Redis-down + `fail_open=true` allows traffic and logs warning |
| [ ] | W3-04 | Implement fail-closed mode behavior | BE | Redis-down + `fail_open=false` blocks RL-protected path as defined |
| [ ] | W3-05 | Update `/ready` degraded semantics for fail-open mode | BE | Redis-down + fail-open returns `200` with degraded payload |
| [ ] | W3-06 | Add Prometheus metrics with low-cardinality defaults | BE | `gateway_http_requests_total` and latency metrics expose route/service labels only |
| [ ] | W3-07 | Add optional high-cardinality tenant metric toggle | BE + PLAT | Tenant metric absent by default and present only when enabled |
| [ ] | W3-08 | Gate C signoff | PM + QA | Redis outage and rate-limit scenarios verified in staging |

---

## Week 4 (Apr 13 - Apr 17): Production Hardening

| Done | ID | Task | Owner | Acceptance Test |
|---|---|---|---|---|
| [ ] | W4-01 | Configure upstream transport pooling and timeouts | BE | Timeout/unreachable upstream path returns 503/504 |
| [ ] | W4-02 | Preserve upstream 4xx/5xx status codes | BE | Upstream 500 response passes through unchanged |
| [ ] | W4-03 | Implement graceful shutdown drain logic | BE | SIGTERM drains in-flight requests within configured timeout |
| [ ] | W4-04 | Add WebSocket upgrade proxy support | BE | WS echo integration test passes through gateway |
| [ ] | W4-05 | Add security headers + CORS suffix-match behavior | BE + SEC | CORS rejects wildcard misuse and sets expected security headers |
| [ ] | W4-06 | Run 24h soak test + goroutine leak check | QA + BE | Goroutine count stable and no leak alarms |
| [ ] | W4-07 | Run baseline load test for NFR targets | QA + PLAT | p50 and p99 overhead targets met in staging |
| [ ] | W4-08 | Gate D signoff | PM + QA | Hardening gate approved with no unresolved critical defects |

---

## Week 5 (Apr 20 - Apr 24): Open Source Packaging

| Done | ID | Task | Owner | Acceptance Test |
|---|---|---|---|---|
| [x] | W5-01 | Add `LICENSE` (AGPLv3) and `LICENSE-COMMERCIAL.md` | PM | Dual-license docs are present and referenced in README |
| [x] | W5-02 | Add `CONTRIBUTING.md`, `CLA.md`, `CODE_OF_CONDUCT.md`, `SUPPORT.md`, `TRADEMARKS.md` | PM | Docs are complete and linked from README |
| [x] | W5-03 | Add `SECURITY.md` disclosure policy | SEC | Security contact and disclosure flow are explicit |
| [x] | W5-04 | Add quickstart and compatibility matrix to README | BE + PLAT | New developer can run gateway locally in < 10 minutes |
| [ ] | W5-05 | Provide sample configs (single-tenant + multi-tenant) | BE | Both example configs boot and route mock service traffic |
| [ ] | W5-06 | Finalize Docker + compose + Kubernetes manifests | PLAT | `docker compose up` works; K8s manifest deploys in test cluster |
| [ ] | W5-07 | Add CI security checks (`gosec`, `govulncheck`) | PLAT + SEC | CI fails on introduced vulnerability and passes on clean main branch |
| [ ] | W5-08 | Gate E signoff | PM + QA + SEC | OSS readiness checklist complete |

---

## Week 6 (Apr 27 - May 1): Pilot and Release

| Done | ID | Task | Owner | Acceptance Test |
|---|---|---|---|---|
| [ ] | W6-01 | Onboard one internal project via config-only integration | BE + PLAT | Project traffic routed through gateway without code fork |
| [ ] | W6-02 | Run canary rollout (10% -> 50% -> 100%) | PLAT | Error rate and latency remain within thresholds at each stage |
| [ ] | W6-03 | Validate JWT key rotation runbook in staging | SEC + BE | Rotation completes without auth downtime |
| [ ] | W6-04 | Validate Redis failure runbook in staging | PLAT + QA | Expected fail-open/closed behaviors observed and documented |
| [ ] | W6-05 | Final defect triage and release blocker closure | PM + QA | No open Sev-1/Sev-2; accepted risk list documented |
| [ ] | W6-06 | Prepare release notes + changelog | PM + BE | Notes include breaking changes, migration guidance, known issues |
| [ ] | W6-07 | Tag and publish `v0.1.0` | PM + PLAT | Release tag and artifacts are publicly available |
| [ ] | W6-08 | Gate F signoff and retrospective | PM + Team | Release signed off and retrospective action items captured |

---

## Cross-Cutting Quality Checklist (Run Weekly)

| Done | Check | Owner | Pass Criteria |
|---|---|---|---|
| [ ] | Unit + integration test suite | QA + BE | Green on CI |
| [ ] | Security scan (`gosec`, `govulncheck`) | SEC | No high/critical unresolved issues |
| [ ] | Metrics cardinality check | PLAT | Series growth within expected bounds |
| [ ] | Dependency update review | BE + SEC | New deps reviewed for license and security |
| [ ] | Documentation drift check | PM | Docs match implemented behavior |
