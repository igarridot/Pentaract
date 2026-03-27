#!/bin/sh
set -eu

# Database restore script for Pentaract.
# Restores a compressed pg_dump backup into the running PostgreSQL instance.
# The application (pentaract service) must be stopped before restoring.
#
# Usage: db-restore.sh <backup_filename_or_path>
#        db-restore.sh --list
#   e.g. db-restore.sh pentaract_20260322_030000.sql.gz

BACKUP_DIR="${BACKUP_DIR:-/backups}"

list_backups() {
  set -- "$BACKUP_DIR"/pentaract_*.sql.gz
  if [ ! -f "$1" ]; then
    echo "  (none)"
    return 0
  fi

  ls -1t "$BACKUP_DIR"/pentaract_*.sql.gz 2>/dev/null | while read -r f; do
    printf "  %s  (%s)\n" "$(basename "$f")" "$(du -h "$f" | cut -f1)"
  done
}

resolve_backup_path() {
  input_path="$1"

  if [ -f "$input_path" ]; then
    printf '%s\n' "$input_path"
    return 0
  fi

  candidate_path="${BACKUP_DIR}/$(basename "$input_path")"
  if [ -f "$candidate_path" ]; then
    printf '%s\n' "$candidate_path"
    return 0
  fi

  return 1
}

if [ $# -lt 1 ]; then
  echo "Usage: db-restore.sh <backup_filename_or_path>"
  echo "       db-restore.sh --list"
  echo ""
  echo "Available backups:"
  list_backups
  exit 1
fi

if [ "$1" = "--list" ]; then
  list_backups
  exit 0
fi

INPUT_PATH="$1"
if ! FILEPATH="$(resolve_backup_path "$INPUT_PATH")"; then
  echo "ERROR: Backup file not found: ${INPUT_PATH}"
  exit 1
fi

FILENAME="$(basename "$FILEPATH")"

echo "[$(date)] Restoring from: ${FILENAME}"

# Drop and recreate the database to ensure a clean restore.
# pg_dump plain format produces a full SQL script that expects an empty database.
echo "[$(date)] Dropping and recreating database ${DATABASE_NAME}..."
psql \
  -h "$DATABASE_HOST" \
  -p "$DATABASE_PORT" \
  -U "$DATABASE_USER" \
  -d postgres \
  --no-password \
  -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${DATABASE_NAME}' AND pid <> pg_backend_pid();" \
  -c "DROP DATABASE IF EXISTS \"${DATABASE_NAME}\";" \
  -c "CREATE DATABASE \"${DATABASE_NAME}\" OWNER \"${DATABASE_USER}\";"

echo "[$(date)] Loading backup into ${DATABASE_NAME}..."
gunzip -c "$FILEPATH" \
  | psql \
    -h "$DATABASE_HOST" \
    -p "$DATABASE_PORT" \
    -U "$DATABASE_USER" \
    -d "$DATABASE_NAME" \
    --no-password \
    -v ON_ERROR_STOP=1 \
    --single-transaction

echo "[$(date)] Restore complete."
