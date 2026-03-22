# Backup And Restore

Use this runbook for the current production shape:

- `api-gateway`
- `collector`
- `portal`
- PostgreSQL
- Redis

## What To Back Up

- PostgreSQL database
- Helm values / Compose env files
- secret material from your secret manager
- exported audit and usage evidence required by compliance

Redis is treated as rebuildable cache/state, not the system of record.

## Local Or VM Backup

Windows:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\backup_postgres.ps1 `
  -DatabaseUrl "postgres://user:pass@host:5432/agentfabric?sslmode=require" `
  -OutputDir .\backups `
  -Format custom `
  -RetentionDays 7
```

Linux/macOS:

```bash
DATABASE_URL="postgres://user:pass@host:5432/agentfabric?sslmode=require" \
OUTPUT_DIR=./backups \
BACKUP_FORMAT=custom \
RETENTION_DAYS=7 \
bash scripts/backup-postgres.sh
```

## Kubernetes Backup

Enable:

- [backup-cronjob.yaml](/C:/Users/vrast/Documents/Agentic%20Code/files/deploy/helm/templates/backup-cronjob.yaml)
- `backups.enabled=true`

Then provide:

- `database-url` in the Helm secret
- a PVC or existing claim for `/backups`

## Restore

Custom-format dump:

```bash
pg_restore --clean --if-exists --no-owner --dbname "$DATABASE_URL" /path/to/agentfabric-<stamp>.dump
```

Plain SQL:

```bash
psql "$DATABASE_URL" -f /path/to/agentfabric-<stamp>.sql
```

## Backup Policy

- run every 6 hours for production
- keep at least 7 days online
- copy encrypted backups to object storage outside the cluster
- test restore monthly
- attach the latest successful restore test to release operations review
