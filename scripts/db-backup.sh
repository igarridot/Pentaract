#!/bin/sh
set -eu

# Database backup script for Pentaract.
# Runs pg_dump and saves a timestamped compressed backup.
# Old backups beyond BACKUP_RETENTION_DAYS are automatically removed.

BACKUP_DIR="${BACKUP_DIR:-/backups}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-7}"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
FILENAME="pentaract_${TIMESTAMP}.sql.gz"

echo "[$(date)] Starting database backup..."

pg_dump \
  -h "$DATABASE_HOST" \
  -p "$DATABASE_PORT" \
  -U "$DATABASE_USER" \
  -d "$DATABASE_NAME" \
  --no-password \
  --no-blobs \
  | gzip -1 > "${BACKUP_DIR}/${FILENAME}"

echo "[$(date)] Backup saved: ${FILENAME} ($(du -h "${BACKUP_DIR}/${FILENAME}" | cut -f1))"

# Remove backups older than retention period
find "$BACKUP_DIR" -name "pentaract_*.sql.gz" -mtime +"$RETENTION_DAYS" -delete

REMAINING=$(find "$BACKUP_DIR" -name "pentaract_*.sql.gz" | wc -l)
echo "[$(date)] Cleanup done. ${REMAINING} backup(s) retained."
