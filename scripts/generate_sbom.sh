#!/usr/bin/env bash
# Generate SPDX + CycloneDX SBOMs via Syft (Docker).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

OUT_DIR="${OUT_DIR:-${ROOT}/artifacts/sbom}"
mkdir -p "${OUT_DIR}"

SYFT_IMAGE="${SYFT_IMAGE:-anchore/syft:v1.20.0}"
VERSION="${VERSION:-$(git describe --tags --abbrev=0 2>/dev/null || echo unknown)}"
SHA="$(git rev-parse --short HEAD)"

SPDX_REL="artifacts/sbom/staff_app_api-${VERSION}-sha-${SHA}.spdx.json"
CDX_REL="artifacts/sbom/staff_app_api-${VERSION}-sha-${SHA}.cdx.json"

echo "Generating SBOM with ${SYFT_IMAGE}"
docker run --rm -v "${ROOT}:/src" -w /src "${SYFT_IMAGE}" dir:/src \
  --exclude './artifacts/**' \
  --exclude './api/**' \
  --exclude './frontend_lab/**' \
  --exclude './docs/**' \
  --exclude './.git/**' \
  -o "spdx-json=${SPDX_REL}" \
  -o "cyclonedx-json=${CDX_REL}"

echo "SBOM artifacts:"
echo "  ${ROOT}/${SPDX_REL}"
echo "  ${ROOT}/${CDX_REL}"
