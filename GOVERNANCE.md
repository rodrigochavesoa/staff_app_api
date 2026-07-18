# Governance

This document describes how STAFF App API is governed.

The goal is to keep decision-making transparent, conservative around public API
contracts, careful around health-adjacent behavior and welcoming to external
contributors.

## Project Scope

STAFF App API is responsible for the backend API only:

- Go HTTP/JSON services;
- OpenAPI contract;
- SQLite migrations and persistence;
- Docker/runtime operation;
- authentication and authorization;
- training, anamnese, Garmin, SVED, RAG and reporting workflows;
- community, release, security and license policy for this backend package.

Out of scope unless explicitly approved:

- official frontend applications;
- legacy Python code under `api/`;
- local design labs under `frontend_lab/`;
- GraphRAG or agentic AI expansion beyond the approved roadmap;
- medical, legal or business advice.

## Roles

### Users

Users run, integrate or evaluate the API. They may open issues, ask questions
and report bugs.

### Contributors

Contributors submit issues, documentation, tests, code, security improvements
or operational fixes. Contributors must follow the Code of Conduct and
Contribution Guidelines.

### Maintainers

Maintainers have authority to triage issues, review PRs, merge changes, manage
releases and enforce project policy.

Maintainers are responsible for:

- protecting the public API contract;
- reviewing security-sensitive changes;
- reviewing dependency and license changes;
- ensuring migrations and operational scripts are safe;
- coordinating releases and advisories;
- enforcing the Code of Conduct.

### Security Maintainers

Security maintainers handle private vulnerability reports and coordinate fixes.
Until a dedicated group exists, active maintainers fill this role.

## Decision-Making

The project uses maintainer consensus for ordinary decisions.

Ordinary changes may be approved through normal PR review:

- bug fixes;
- documentation updates;
- tests;
- small implementation improvements;
- OpenAPI corrections that match existing behavior.

Major changes require a public issue or proposal before implementation:

- new public API domains;
- breaking API changes;
- authentication or authorization changes;
- new AI provider behavior;
- new storage engines;
- schema changes with migration risk;
- dependency changes with licensing, security or operational impact;
- release process changes;
- license or governance changes.

If consensus cannot be reached, maintainers may defer the decision, request a
smaller proposal or reject the change until the risk is better understood.

## API Stability

While the project is in `v0.x`, breaking changes may happen, but they must be:

- intentional;
- documented in `CHANGELOG.md`;
- reflected in `openapi.yaml`;
- justified in the PR or release notes.

The project should reserve `v1.0.0` for a stable public contract and production
operational posture.

## Release Governance

Before a public release, maintainers should verify:

- tests, race detector, vet and build pass;
- staticcheck, gosec and govulncheck pass;
- TruffleHog reports no verified or unknown secrets;
- OpenAPI lint has no errors or warnings under the project ruleset;
- Docker healthcheck passes;
- smoke test and E2E API journey pass;
- license policy and bundled assets are reviewed.

Release tags should follow SemVer. In the dedicated repository strategy, tags
use direct form such as `v0.1.0`.

## Security and Emergency Changes

Security fixes may be handled privately until a patch is ready. Maintainers may
merge urgent security fixes with abbreviated review when delay would increase
user risk. A follow-up review and documentation update should happen after the
fix.

## Becoming a Maintainer

Maintainer status may be granted to contributors who demonstrate:

- consistent high-quality contributions;
- respect for the API-first and frontend-agnostic scope;
- good security and privacy judgment;
- constructive review behavior;
- understanding of Go, SQLite, OpenAPI and operational workflows.

Maintainer access may be removed for inactivity, repeated policy violations,
security mishandling or conduct violations.

## Conflict Resolution

Technical disagreements should be resolved with evidence:

- tests;
- benchmarks when relevant;
- security analysis;
- OpenAPI contract impact;
- migration and operational risk;
- maintainability.

Conduct concerns are handled under [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
Security concerns are handled under [SECURITY.md](SECURITY.md).
