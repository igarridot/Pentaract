#!/bin/sh
set -eu

# Entrypoint for the db-backup container.
# Runs the backup script once immediately, then every BACKUP_INTERVAL_SECONDS
# (default: 86400 = 24 hours).

if [ "$#" -gt 0 ]; then
  exec "$@"
fi

INTERVAL="${BACKUP_INTERVAL_SECONDS:-86400}"

echo "[$(date)] Backup scheduler started. Interval: ${INTERVAL}s"

while true; do
  /usr/local/bin/db-backup.sh || echo "[$(date)] WARNING: backup failed"
  echo "[$(date)] Next backup in ${INTERVAL}s"
  sleep "$INTERVAL"
done
