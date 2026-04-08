#!/usr/bin/env bash
set -euo pipefail

if ! command -v pg_dump >/dev/null 2>&1; then
  echo "pg_dump was not found in PATH. Install PostgreSQL client tools first." >&2
  exit 1
fi

DATABASE_URL="${DATABASE_URL:-}"
OUTPUT_DIR="${OUTPUT_DIR:-./backups}"
BACKUP_FORMAT="${BACKUP_FORMAT:-custom}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"

if [[ -z "${DATABASE_URL}" ]]; then
  echo "DATABASE_URL is required" >&2
  exit 1
fi

mkdir -p "${OUTPUT_DIR}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
EXT="dump"
case "${BACKUP_FORMAT}" in
  plain) EXT="sql" ;;
  directory) EXT="dir" ;;
  tar) EXT="tar" ;;
esac

TARGET="${OUTPUT_DIR}/govagn-${STAMP}.${EXT}"
pg_dump "${DATABASE_URL}" --format="${BACKUP_FORMAT}" --file="${TARGET}"
find "${OUTPUT_DIR}" -type f -mtime +"${RETENTION_DAYS}" -delete
echo "Backup created at ${TARGET}"
