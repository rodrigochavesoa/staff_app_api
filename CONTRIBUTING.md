# Contributing to STAFF App API

Thank you for considering a contribution to STAFF App API.

This project is a frontend-agnostic Go backend for training, student
management, anamnese workflows, Garmin activity processing, exercise libraries,
SVED metrics, RAG-assisted knowledge retrieval and administrative reporting.

Contributions are welcome, but this project handles health-adjacent training
data. Please treat code, documentation, test data and operational decisions with
extra care.

## Project Principles

- **API first**: public behavior is defined by HTTP/JSON contracts and
  `openapi.yaml`.
- **Frontend agnostic**: do not couple backend code to Flutter, React, HTML
  templates, mobile clients or any single UI.
- **Deterministic core**: AI providers may assist, but safe local behavior must
  remain testable and predictable.
- **Security by default**: no secrets, real databases, real uploads or personal
  data in commits, issues, logs or test fixtures.
- **Small, reviewable changes**: prefer focused PRs over broad rewrites.
- **Operational clarity**: migrations, backup/restore behavior, Docker runtime
  paths and environment variables must be explicit.

## Before You Start

1. Read the [README](README.md).
2. Review the API contract in [openapi.yaml](openapi.yaml).
3. Check existing issues and pull requests.
4. For large changes, open an issue or design proposal before implementation.

Large changes include:

- new public endpoints;
- database schema changes;
- auth, role or permission changes;
- AI provider behavior;
- upload or filesystem behavior;
- changes that may affect privacy, safety or licensing.

## Development Setup

```bash
cp .env.example .env
GOCACHE=/tmp/go-build-cache GOWORK=off go run ./cmd/api
```

Docker local staging:

```bash
docker compose build
docker compose up -d
curl -s http://localhost:5000/health
```

## Pull Request Checklist

Before opening a PR, run the relevant local checks:

```bash
GOCACHE=/tmp/go-build-cache GOWORK=off go test ./...
GOCACHE=/tmp/go-build-cache GOWORK=off go test -race ./...
GOCACHE=/tmp/go-build-cache GOWORK=off go vet ./...
GOCACHE=/tmp/go-build-cache GOWORK=off go build ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
go run github.com/securego/gosec/v2/cmd/gosec@v2.22.10 ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Secret scanning:

```bash
docker run --rm -v "$PWD:/repo" trufflesecurity/trufflehog:3.95.9 filesystem /repo --results=verified,unknown --fail
```

When Docker is available, also run:

```bash
./scripts/smoke_test.sh http://localhost:5000
./scripts/e2e_api_journey.sh http://localhost:5000
```

## API Contract Rules

Update `openapi.yaml` whenever a PR changes:

- route paths;
- HTTP methods;
- request or response payloads;
- status codes;
- authentication or authorization requirements;
- query or path parameters.

Lint the contract:

```bash
/tmp/go-tools/vacuum lint -r vacuum-ruleset.yaml openapi.yaml
```

JSON fields intentionally use `snake_case` for compatibility. Do not rename
fields only to satisfy generic style preferences.

## Database and Migrations

- Add forward migrations for schema changes.
- Keep migrations safe for existing SQLite files whenever possible.
- Do not remove user data automatically.
- Prefer idempotent or Go-aware migration handling when legacy databases may
  already contain a column or table.
- Add repository and integration tests for persistence behavior.

## AI and RAG Contributions

AI integrations must preserve these rules:

- local deterministic behavior remains available;
- provider failures are handled explicitly;
- no real API keys are used in tests;
- safety validators cannot be bypassed by provider output;
- generated plans must expose whether AI was used through metadata.

Use fake providers and anonymized fixtures for tests.

## Test Data and Privacy

Never commit:

- `.env` files;
- API keys or credentials;
- real SQLite databases;
- real Garmin uploads;
- production logs;
- personally identifiable health or student data;
- proprietary third-party documents without permission.

Fixtures must be synthetic or anonymized.

## Licensing

STAFF App API is licensed under Apache-2.0. New dependencies must follow
[LICENSE_POLICY.md](LICENSE_POLICY.md).

Do not introduce GPL, AGPL, SSPL, Commons Clause, non-commercial or proprietary
dependencies without maintainer approval and a documented license review.

## Medical and Training Safety

This project provides software infrastructure for training workflows. It is not
a substitute for professional medical, clinical or coaching judgment.

Contributions that affect training generation, anamnese risk, exercise
contraindications or safety scoring must include tests and explain the safety
reasoning in the PR.

## Commit and PR Style

### Conventional Commits

All commits must follow [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>(optional-scope): <description>

[optional body]

[optional footer(s)]
```

Rules:

- Use an imperative, concise description (≤ ~72 characters on the subject line).
- Do not end the subject with a period.
- Use `type(scope):` only when a scope helps (examples: `api`, `garmin`, `docs`,
  `ci`, `docker`). Scope is optional.
- Breaking changes: add `!` after the type/scope (`feat(api)!: ...`) and/or a
  footer `BREAKING CHANGE: <details>`. Call out breaking API behavior in the PR
  as well.

Allowed types:

| Type | When to use |
|------|-------------|
| `feat` | New user-facing capability |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `refactor` | Code change with no intended behavior change |
| `test` | Tests only |
| `chore` | Maintenance (deps, tooling, housekeeping) |
| `ci` | CI/CD workflow changes |
| `build` | Build system, Docker image packaging, scripts that produce artifacts |
| `perf` | Performance improvement |
| `style` | Formatting only (no logic change) |

Examples:

```text
feat(api): add public running calendar endpoint
fix(garmin): reject empty FIT uploads before parse
docs: document local Swagger UI and Redoc
ci: pin TruffleHog image tag in workflow
chore(licenses): add module license allowlist gate
feat(api)!: rename training sheet status field

BREAKING CHANGE: clients must send `status` instead of `estado`.
```

### Pull requests

- Keep PRs focused.
- Include screenshots only for optional frontend experiments, not for backend
  behavior.
- Mention breaking API behavior explicitly.
- Link issues with `Closes #123` when applicable.
- Prefer Conventional Commit subjects for the PR title when practical.

## Review and Merge Policy

Maintainers may ask for changes when a PR:

- changes public contracts without updating OpenAPI;
- reduces test coverage around sensitive behavior;
- introduces unreviewed dependencies;
- commits secrets or real data;
- couples backend behavior to a specific frontend;
- bypasses security, safety or license policy.

By contributing, you agree that your contribution will be licensed under the
project license.
