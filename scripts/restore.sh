#!/bin/bash
set -e

# Restore script for SQLite database in docker compose environment
if [ -z "$1" ]; then
  echo "Usage: $0 <path_to_backup_db_file>"
  exit 1
fi

BACKUP_FILE="$1"

if [ ! -f "$BACKUP_FILE" ]; then
  echo "Error: Backup file '$BACKUP_FILE' not found."
  exit 1
fi

echo "Warning: Restoring the database will overwrite the current live database."
read -p "Are you sure you want to proceed? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Restore cancelled."
    exit 0
fi

# Ensure the container is restarted if stopped, even in case of error/interruption
API_STOPPED=false
cleanup() {
  if [ "$API_STOPPED" = true ]; then
    echo "Emergency restoration cleanup: Attempting to restart STAFF API container..."
    docker compose start api || echo "Warning: Failed to restart STAFF API container. Please check status manually."
  fi
}
trap cleanup EXIT

# 1. Create a safety backup of the active database before overwriting
echo "Creating safety backup of the current database before restore..."
docker run --rm \
  -v staff_app_sqlite_data:/data \
  alpine:latest \
  sh -c "[ -f /data/fichas_treino.db ] && cp /data/fichas_treino.db /data/fichas_treino.db.pre-restore.bak && echo 'Safety backup created as /data/fichas_treino.db.pre-restore.bak' || echo 'No existing database to backup.'"

# 2. Stop the API container to prevent open file locks during overwrite
echo "Stopping STAFF API container..."
docker compose stop api
API_STOPPED=true

# 3. Restore database file from backup into the volume
echo "Restoring database from $BACKUP_FILE..."

# Copy backup file into the volume using a temporary container.
# Dynamically match the UID/GID of the parent volume directory to avoid permissions issues.
docker run --rm \
  -v staff_app_sqlite_data:/data \
  -v "$(realpath "$BACKUP_FILE"):/backup.db" \
  alpine:latest \
  sh -c "cp /backup.db /data/fichas_treino.db && chown \$(stat -c '%u:%g' /data) /data/fichas_treino.db"

# 4. Restart container and release the trap
echo "Starting STAFF API container..."
docker compose start api
API_STOPPED=false

echo "Database restore completed successfully."
