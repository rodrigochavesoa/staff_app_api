# Support

This document explains how to get help with STAFF App API.

## What This Project Supports

STAFF App API is a Go backend and HTTP/JSON API. Support is focused on:

- local setup;
- Docker Compose execution;
- API contract usage;
- authentication and JWT flow;
- migrations and SQLite operation;
- Garmin FIT/CSV processing;
- OpenAPI, smoke tests and E2E API journey;
- contribution workflow.

This repository does not provide professional medical, clinical, coaching or
legal advice.

## Before Asking for Help

Please check:

- [README.md](README.md) for setup and common commands;
- [openapi.yaml](openapi.yaml) for routes, payloads and auth requirements;
- [SECURITY.md](SECURITY.md) if the issue may expose a vulnerability;
- existing GitHub issues and discussions.

## Where to Ask

Use GitHub Issues for:

- reproducible bugs;
- contract mismatches;
- Docker or migration failures;
- documentation corrections;
- feature proposals with clear scope.

Use GitHub Discussions for:

- usage questions;
- integration ideas;
- frontend experiments;
- architecture discussion that is not ready for an issue.

Use the private security reporting channel for:

- suspected vulnerabilities;
- secrets exposure;
- auth bypasses;
- path traversal;
- unsafe upload behavior;
- exposure of personal, training or health-related data.

## What to Include

For setup or bug reports, include:

- operating system;
- Go version;
- Docker version, if applicable;
- commit or release version;
- command executed;
- sanitized request/response payload;
- relevant logs with secrets removed;
- whether the issue reproduces with Docker.

## What Not to Include

Do not include:

- real `.env` files;
- API keys;
- passwords;
- real SQLite databases;
- real Garmin uploads;
- student, coach, patient or health data;
- proprietary content from external sources.

## Response Expectations

This is an open source project. Maintainers and contributors respond on a
best-effort basis. Clear reproduction steps and sanitized examples receive
better and faster responses.
