# GitHub Repository Setup (Professional Baseline)

Use this checklist immediately after creating the remote repository.

## 1. Repository Metadata

In `Settings -> General`:

- Set repository description (short and clear).
- Add topics: `api-gateway`, `golang`, `gateway`, `multi-tenant`, `reverse-proxy`.
- Set website URL if docs or project page exists.
- Keep default branch as `main`.

## 2. Branch Protection

In `Settings -> Branches -> Add rule` for `main`:

- Require a pull request before merging.
- Require approvals: minimum `1` (or `2` when team grows).
- Dismiss stale pull request approvals when new commits are pushed.
- Require status checks to pass before merging.
- Required status checks: `test` (from `.github/workflows/ci.yml`).
- Require conversation resolution before merging.
- Restrict force pushes.
- Restrict deletions.

## 3. Merge Strategy

In `Settings -> General -> Pull Requests`:

- Enable squash merging.
- Disable merge commits (optional but cleaner history).
- Disable rebase merging (optional if you standardize on squash).
- Automatically delete head branches after merge.

## 4. Security Settings

In `Settings -> Security`:

- Enable dependency graph.
- Enable Dependabot alerts.
- Enable Dependabot security updates.
- Enable secret scanning (if available for your plan).
- Enable push protection for secrets (if available).

## 5. Discussions, Issues, and Templates

- Enable Issues.
- Enable Discussions (recommended for open-source support).
- Keep issue templates enabled from `.github/ISSUE_TEMPLATE`.
- Keep blank issues disabled (already configured).

## 6. Community and Governance

Confirm the following files exist in default branch root:

- `README.md`
- `LICENSE`
- `LICENSE-COMMERCIAL.md`
- `CONTRIBUTING.md`
- `CODE_OF_CONDUCT.md`
- `SECURITY.md`
- `SUPPORT.md`
- `CLA.md`
- `TRADEMARKS.md`
- `CODEOWNERS`

## 7. Required Placeholder Replacements

Before public launch, verify:

- `.github/CODEOWNERS` points to your active maintainer account/team
- `.github/ISSUE_TEMPLATE/config.yml` points to your live security advisory URL
- policy/license contact emails are reachable (`support@gogatehq.dev`)

## 8. Release Hygiene

- Create first tag as `v0.1.0` when ready.
- Create a GitHub release with notes referencing `CHANGELOG.md`.
- Use semantic versioning: `MAJOR.MINOR.PATCH`.

## 9. First Push Flow

```bash
git add .
git commit -m "chore: bootstrap gateway and open-source project foundation"
git branch -M main
git remote add origin <your-repo-url>
git push -u origin main
```
