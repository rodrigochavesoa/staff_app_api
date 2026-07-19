#!/usr/bin/env bash
# Start/stop local Swagger UI + Redoc against ./openapi.yaml (no paid SaaS).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

COMPOSE=(docker compose -f docker-compose.docs.yml)
DOCS_PORT="${DOCS_PORT:-8080}"
REDOC_PORT="${REDOC_PORT:-8081}"

usage() {
  cat <<EOF
Usage: $(basename "$0") <up|down|restart|status|url>

  up       Start Swagger UI (:${DOCS_PORT}) and Redoc (:${REDOC_PORT})
  down     Stop docs containers
  restart  Recreate docs containers
  status   Show compose status
  url      Print local URLs

Requires Docker. Does not start the API — run the API separately for Try it out:

  docker compose up -d
  # API:  http://localhost:5000
  # Docs: http://localhost:${DOCS_PORT}  (Swagger UI)
  #       http://localhost:${REDOC_PORT}  (Redoc)

No SwaggerHub account, swagger.io login, VPS, or GHCR required.
EOF
}

cmd="${1:-}"
case "${cmd}" in
  up)
    if [[ ! -f openapi.yaml ]]; then
      echo "openapi.yaml not found in ${ROOT}" >&2
      exit 1
    fi
    "${COMPOSE[@]}" up -d
    echo
    echo "Swagger UI: http://localhost:${DOCS_PORT}"
    echo "Redoc:      http://localhost:${REDOC_PORT}"
    echo "API Try it out target (from openapi.yaml): http://localhost:5000"
    echo
    echo "Start the API in another terminal if you want live requests:"
    echo "  docker compose up -d"
    ;;
  down)
    "${COMPOSE[@]}" down
    ;;
  restart)
    "${COMPOSE[@]}" up -d --force-recreate
    echo "Swagger UI: http://localhost:${DOCS_PORT}"
    echo "Redoc:      http://localhost:${REDOC_PORT}"
    ;;
  status)
    "${COMPOSE[@]}" ps
    ;;
  url|urls)
    echo "http://localhost:${DOCS_PORT}"
    echo "http://localhost:${REDOC_PORT}"
    ;;
  -h|--help|help|"")
    usage
    if [[ -z "${cmd}" ]]; then
      exit 1
    fi
    ;;
  *)
    echo "Unknown command: ${cmd}" >&2
    usage
    exit 1
    ;;
esac
