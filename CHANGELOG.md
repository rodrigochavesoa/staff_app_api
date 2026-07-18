# Changelog

All notable changes to STAFF App API are documented in this file.

The format is based on Keep a Changelog 1.1.0, and this project follows
Semantic Versioning.

## [Unreleased]

### Planned

- Public repository packaging for the dedicated `staff_app` repository.
- First annotated release tag `v0.1.0`.
- Remote CI confirmation on the public repository.
- Versioned Docker image tags.

### Not Included

- Official frontend applications.
- Legacy Python reference code.
- Local documentation under `docs/`.
- Local Flutter/design experiments under `frontend_lab/`.
- Real AI provider credentials.

## [0.1.0-rc] - 2026-07-18

Local release candidate approved after API-only validation.

### Added

- Auth/JWT, bootstrap admin, users, approval flow and plans.
- Student CRUD, search, frequency and training history.
- Public pre-registration, anamnese tokens, anamnese submission, clinical
  approval/rejection and operational e-mail workflow.
- Manual and periodized training sheets with public links, feedback and
  optimistic concurrency control.
- Running periodization, Daniels-style deterministic blocks, monthly calendar
  endpoints and public running plan links.
- Garmin FIT/CSV upload, records, analytics and Plotly-compatible chart data.
- Exercise library from CSV plus custom therapeutic exercises and suggestion
  moderation.
- SVED calculation, batch processing, history, suggestions and dashboards.
- Knowledge base/RAG endpoints with SQLite cache and local fallback.
- Config management, SMTP test, dashboard metrics and specialized admin
  reports.
- Multi-provider AI training interface with deterministic fallback and metadata
  showing whether AI was used.
- OpenAPI 3 contract with 124 operations.
- Docker Compose local staging, non-root runtime and persistent volumes.
- Backup, restore, smoke test and E2E API-only journey scripts.
- Apache-2.0 license, community files and license policy.

### Changed

- Public API package is prepared for the dedicated repository strategy.
- Exercise HTML/GIF assets remain external through direct static URLs instead
  of a backend HTML proxy.
- JSON contracts intentionally preserve `snake_case` compatibility.

### Security

- Production startup rejects default development secrets.
- Upload size and path traversal protections are enforced.
- TruffleHog, gosec, govulncheck and staticcheck are part of the release gate.
- Local RC validation found no verified or unknown secrets.

### Validation

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `go build ./...`
- `staticcheck`
- `gosec` with 0 issues
- `govulncheck` with no vulnerabilities in called code
- `vacuum` OpenAPI lint with 0 errors and 0 warnings under the project ruleset
- Docker build, healthcheck, smoke test and E2E API-only journey
