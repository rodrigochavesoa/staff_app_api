## Summary

Describe what this PR changes and why.

## Related Issues

Closes #

## Change Type

- [ ] Bug fix
- [ ] Backend/API feature
- [ ] Database migration
- [ ] Documentation
- [ ] Tests
- [ ] Security hardening
- [ ] Refactor without behavior change
- [ ] Release or packaging

## Public API Contract

- [ ] Does not change public routes, payloads or status codes
- [ ] Changes public API behavior and updates `openapi.yaml`
- [ ] Updates auth/role requirements and documents them
- [ ] Not applicable

## Database

- [ ] Does not change schema
- [ ] Adds a migration
- [ ] Migration is safe for existing SQLite databases
- [ ] Does not delete user data automatically
- [ ] Not applicable

## Validation

- [ ] `go test ./...`
- [ ] `go test -race ./...`
- [ ] `go vet ./...`
- [ ] `go build ./...`
- [ ] `staticcheck`
- [ ] `gosec`
- [ ] `govulncheck`
- [ ] OpenAPI lint with `vacuum-ruleset.yaml`
- [ ] Smoke or E2E API journey, when applicable

## Security, Privacy and Safety

- [ ] No `.env`, secrets, databases, uploads, logs or real personal data
- [ ] No real health, student, coach or Garmin data
- [ ] No new dependency with unknown or restricted license
- [ ] Upload, path, auth, role and SQL behavior are tested when touched
- [ ] Training/anamnese/AI changes preserve deterministic safety rules

## Frontend Scope

- [ ] Backend remains frontend-agnostic
- [ ] No coupling to a specific UI framework
- [ ] Not applicable

## Notes for Reviewers

Add context, risks, migration notes or follow-up work.
