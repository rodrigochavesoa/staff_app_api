#!/bin/bash
# API-only end-to-end journey against a live staff_app instance (local Docker RC).
# Does not replace smoke_test.sh (short staging sanity); this is full operational homologation.
#
# Usage:
#   ./scripts/e2e_api_journey.sh [API_URL]
# Default API_URL: http://localhost:5000
#
# Optional env:
#   E2E_ADMIN_USERNAME / E2E_ADMIN_PASSWORD
#   ADMIN_DEFAULT_USERNAME / ADMIN_DEFAULT_PASSWORD (fallback)
#   E2E_FIT_FILE
#   E2E_EXPECT_SMTP_FAILURE   # unset=accept fail|success; true=require fail; false=require success
#   E2E_EXPECT_AI_REQUIRED_503=1  # require 503 from gerar-blocos (server must use AI_TRAINING_MODE=required)

set -euo pipefail

HOST="${1:-http://localhost:5000}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export GOCACHE="${GOCACHE:-/tmp/go-build-cache}"
export GOWORK="${GOWORK:-off}"

echo "Running E2E API journey via go run ./cmd/e2eapi against: $HOST"
exec go run ./cmd/e2eapi "$HOST"
