#!/bin/bash
set -e

# Backup script for SQLite database in docker compose environment
BACKUP_DIR="./backups"
mkdir -p "$BACKUP_DIR"

echo "Starting SQLite consistent database backup..."

# Use a temporary alpine container with sqlite3 installed to perform a hot backup.
# This ensures transactional consistency without stopping the main application.
docker run --rm \
  -v staff_app_sqlite_data:/data \
  -v "$(pwd)/$BACKUP_DIR:/backups" \
  alpine:latest \
  sh -c "apk add --no-cache sqlite >/dev/null && sqlite3 /data/fichas_treino.db \".backup /backups/backup_\$(date +%Y%m%d_%H%M%S).db\""

echo "Backup created successfully in '$BACKUP_DIR' directory."
