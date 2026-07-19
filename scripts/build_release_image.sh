#!/usr/bin/env bash
# Build a versioned staff_app API image with semantic + SHA tags.
# Does not push. Set IMAGE_REGISTRY to prefix for a future push target, e.g.:
#   IMAGE_REGISTRY=ghcr.io/rodrigochavesoa ./scripts/build_release_image.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-$(git describe --tags --abbrev=0 2>/dev/null || true)}"
if [[ -z "${VERSION}" ]]; then
  echo "VERSION is unset and no git tag was found. Export VERSION=vX.Y.Z" >&2
  exit 1
fi

SHA="$(git rev-parse --short HEAD)"
# Default matches GitHub repo / GHCR package name (underscore, not hyphen).
IMAGE_NAME="${IMAGE_NAME:-staff_app_api}"
REGISTRY_PREFIX="${IMAGE_REGISTRY:-}"
if [[ -n "${REGISTRY_PREFIX}" ]]; then
  REGISTRY_PREFIX="${REGISTRY_PREFIX%/}/"
fi

TAG_VERSION="${REGISTRY_PREFIX}${IMAGE_NAME}:${VERSION}"
TAG_SHA="${REGISTRY_PREFIX}${IMAGE_NAME}:sha-${SHA}"

echo "Building ${TAG_VERSION}"
echo "         ${TAG_SHA}"
echo "Commit:  $(git rev-parse HEAD)"

docker build \
  --label "org.opencontainers.image.title=staff_app_api" \
  --label "org.opencontainers.image.version=${VERSION}" \
  --label "org.opencontainers.image.revision=$(git rev-parse HEAD)" \
  --label "org.opencontainers.image.source=https://github.com/rodrigochavesoa/staff_app_api" \
  -t "${TAG_VERSION}" \
  -t "${TAG_SHA}" \
  .

echo
echo "Local tags ready:"
echo "  ${TAG_VERSION}"
echo "  ${TAG_SHA}"
echo
echo "Push only after the registry destination is authorized, for example:"
echo "  docker push ${TAG_VERSION}"
echo "  docker push ${TAG_SHA}"
