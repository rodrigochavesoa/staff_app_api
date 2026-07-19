#!/usr/bin/env bash
# Module license inventory + allowlist gate (see LICENSE_POLICY.md / docs/license.md).
# Uses a small Go helper because google/go-licenses currently fails on Go 1.25 stdlib packages.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

OUT_DIR="${OUT_DIR:-${ROOT}/artifacts/licenses}"
export OUT_DIR
# Same default as e2e_api_journey.sh / README — avoids read-only module cache dirs in some envs.
export GOCACHE="${GOCACHE:-/tmp/go-build-cache}"
mkdir -p "${OUT_DIR}" "${GOCACHE}"

echo "Scanning Go module licenses..."
GOWORK=off go run "${ROOT}/scripts/check_module_licenses.go"
