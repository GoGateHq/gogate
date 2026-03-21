# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and this project follows Semantic Versioning.

## [Unreleased]

### Changed
- **Breaking:** Go module path renamed from `github.com/opportunation/api-gateway` to `github.com/gogatehq/gogate`.

### Migration Notes
- Update imports in downstream projects:
  - from: `github.com/opportunation/api-gateway/...`
  - to: `github.com/gogatehq/gogate/...`
- Run module update commands in consuming repositories:
  - `go get github.com/gogatehq/gogate@latest`
  - `go mod tidy`

### Added
- Initial gateway scaffold with routing, middleware, health/readiness endpoints, config validation, tests, and CI.
- Open-source governance/legal files (license, contribution, security, support, CLA, trademark policy).
- JWT verification with `kid` key selection, multi-key rotation, and optional JWKS-backed key lookup.
- Tenant resolution strategies (subdomain, header, path) with tenant-aware route enforcement.
- Trusted proxy client-IP resolution and spoofed identity header stripping before upstream forwarding.
