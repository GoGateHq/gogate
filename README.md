# GoGate

GoGate is a config-driven API gateway written in Go.
It is designed to be a reusable edge layer for multiple projects, with centralized routing, auth, tenancy, rate limiting, and observability.

## Project Status

GoGate is currently in active pre-release development (`v0.x`).
Core functionality is being built against the milestones in [ROADMAP.md](./ROADMAP.md).

## Quick Start

Requirements:

- Go 1.22+

Run locally:

```bash
make run
```

Build:

```bash
make build
```

Run tests:

```bash
make test
```

## Development

- Delivery plan: [ROADMAP.md](./ROADMAP.md)
- Execution board: [EXECUTION_CHECKLIST.md](./EXECUTION_CHECKLIST.md)
- Product requirements: [PRD.md](./PRD.md)
- Changelog: [CHANGELOG.md](./CHANGELOG.md)
- Repo setup checklist: [docs/REPO_SETUP.md](./docs/REPO_SETUP.md)

## Compatibility

| Component | Supported |
|---|---|
| Go | 1.22+ |
| Redis | 6+ (for rate limiting phases) |
| Linux/macOS | Supported |

## License

GoGate uses a dual-license model.

- Community use is licensed under **GNU AGPLv3**. See [LICENSE](./LICENSE).
- Commercial use without AGPL obligations requires a separate license. See [LICENSE-COMMERCIAL.md](./LICENSE-COMMERCIAL.md).

Contact: `gogate@youremail.com` (replace with your real project contact email before public launch)

## Contribution and Governance

- Contribution guide: [CONTRIBUTING.md](./CONTRIBUTING.md)
- Contributor licensing terms: [CLA.md](./CLA.md)
- Security policy: [SECURITY.md](./SECURITY.md)
- Support policy: [SUPPORT.md](./SUPPORT.md)
- Trademark policy: [TRADEMARKS.md](./TRADEMARKS.md)
- Code of conduct: [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md)

## Maintainer Notes

Before the first public push, replace placeholder values in:

- [LICENSE-COMMERCIAL.md](./LICENSE-COMMERCIAL.md)
- [SECURITY.md](./SECURITY.md)
- [SUPPORT.md](./SUPPORT.md)
- [TRADEMARKS.md](./TRADEMARKS.md)
- [docs/REPO_SETUP.md](./docs/REPO_SETUP.md)
- [.github/CODEOWNERS](./.github/CODEOWNERS)
- [.github/ISSUE_TEMPLATE/config.yml](./.github/ISSUE_TEMPLATE/config.yml)
