# Security Policy

## Supported Versions

STAFF App API is currently preparing its first public release.

| Version | Status | Security support |
|---|---|---|
| `v0.1.x` | Initial public line | Supported after publication |
| `v0.1.0-rc` | Local release candidate | Best-effort local validation |
| `< v0.1.0` | Pre-release development | Not supported |

Until the public repository and release process are active, security handling is
best-effort and coordinated directly with maintainers.

## Reporting a Vulnerability

Do not open a public issue for suspected vulnerabilities.

Use the private reporting channel configured by the maintainers in the public
repository. If GitHub private vulnerability reporting is enabled, use that
channel first.

Include:

- affected version, commit or Docker image tag;
- clear impact statement;
- reproduction steps with sanitized data;
- affected endpoint, migration, script or configuration;
- whether authentication is required;
- whether personal, health, credential or uploaded data may be exposed;
- suggested fix, if known.

Do not include:

- real API keys;
- real `.env` files;
- production databases;
- real Garmin/FIT uploads;
- student, coach or patient data;
- exploit details in a public issue, PR or discussion.

## Response Expectations

Maintainers aim to:

1. acknowledge valid reports within 2 business days;
2. assess severity and affected versions;
3. prepare a fix or mitigation;
4. coordinate disclosure timing with the reporter when appropriate;
5. publish release notes and advisories for confirmed vulnerabilities.

These timelines are goals, not contractual commitments.

## Security Scope

In scope:

- authentication and authorization bypasses;
- JWT handling flaws;
- admin-only route exposure;
- SQL injection or unsafe migration behavior;
- path traversal or unsafe file upload handling;
- secret exposure;
- unsafe Docker/runtime defaults;
- vulnerabilities in called dependencies;
- OpenAPI contract drift that causes unsafe client behavior;
- privacy leaks involving anamnese, students, uploads or logs.

Out of scope for security reports:

- missing features;
- unsupported legacy Python reference code under `api/`;
- local-only `docs/` or `frontend_lab/` experiments;
- social engineering against maintainers;
- denial-of-service reports without a concrete security impact;
- vulnerability reports that require access to real private data not provided
  by the reporter.

## Maintainer Security Checklist

Before public releases, maintainers should run:

```bash
GOCACHE=/tmp/go-build-cache GOWORK=off go test -race ./...
GOCACHE=/tmp/go-build-cache GOWORK=off go vet ./...
GOCACHE=/tmp/go-build-cache GOWORK=off go build ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
go run github.com/securego/gosec/v2/cmd/gosec@v2.22.10 ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
docker run --rm -v "$PWD:/repo" trufflesecurity/trufflehog:3.95.9 filesystem /repo --results=verified,unknown --fail
```

When the project is hosted on GitHub, maintainers should enable:

- Dependabot alerts;
- secret scanning;
- push protection;
- private vulnerability reporting;
- branch protection with required CI checks.

## Sensitive Data Policy

The repository must not contain:

- `.env`;
- database files;
- backups;
- logs;
- uploads;
- real training, anamnese or Garmin data;
- external proprietary reference materials.

Test fixtures must be synthetic or anonymized.

## Medical and Training Disclaimer

Security reports related to training generation, anamnese risk, exercise
contraindications or AI suggestions are welcome when they affect software
safety. This project does not provide medical advice and should not replace
qualified professional judgment.
