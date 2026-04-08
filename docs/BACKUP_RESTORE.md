# Backup and Restore

## Purpose

This runbook defines the backup and restore approach for the current Govagn production shape.

Use it to answer:

- what must be backed up
- what can be rebuilt
- how backups are taken
- how restores are executed
- what evidence is required for production readiness

## Current Production Shape

This runbook applies to:

- `api-gateway`
- `collector`
- `portal`
- PostgreSQL
- Redis

## What Must Be Backed Up

- PostgreSQL database
- Helm values or Compose environment files
- secret material from your secret manager
- exported audit and usage evidence required by compliance or release review

Redis should be treated as rebuildable runtime cache and coordination state, not the primary system of record.

## Backup Scope by Data Type

### PostgreSQL
PostgreSQL is the critical persistent store for:

- traces and spans
- policies and pricing rules
- prompts and releases
- eval results
- audit records
- user and tenant metadata

### Configuration
Retain a versioned copy of:

- deployment configuration
- Helm values
- environment files
- ingress and TLS configuration references

### Secret Material
Retain secure recovery access for:

- JWT secrets
- vault key material
- OIDC configuration secrets
- database credentials
- gateway auth tokens

Do not store these in plaintext runbooks or casual shared folders.

## Local or VM Backup

### Windows

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\backup_postgres.ps1 `
  -DatabaseUrl "postgres://user:pass@host:5432/govagn?sslmode=require" `
  -OutputDir .\backups `
  -Format custom `
  -RetentionDays 7
```

### Linux or macOS

```bash
DATABASE_URL="postgres://user:pass@host:5432/govagn?sslmode=require" \
OUTPUT_DIR=./backups \
BACKUP_FORMAT=custom \
RETENTION_DAYS=7 \
bash scripts/backup-postgres.sh
```

## Kubernetes Backup

Enable:

- [../deploy/helm/templates/backup-cronjob.yaml](../deploy/helm/templates/backup-cronjob.yaml)
- `backups.enabled=true`

Then provide:

- `database-url` in the Helm secret
- a PVC or existing claim for `/backups`

Recommended practice:

- run encrypted backups on a schedule
- push a copy outside the cluster
- verify retention and cleanup behavior

## Restore

### Custom-format dump

```bash
pg_restore --clean --if-exists --no-owner --dbname "$DATABASE_URL" /path/to/govagn-<stamp>.dump
```

### Plain SQL dump

```bash
psql "$DATABASE_URL" -f /path/to/govagn-<stamp>.sql
```

## Restore Expectations

After restore, validate:

- gateway health and readiness
- collector health and readiness
- portal login
- traces are visible
- pricing and policy data are present
- prompt and eval metadata load correctly
- audit history is intact as expected

## Backup Policy

Recommended baseline:

- run every 6 hours for production
- keep at least 7 days online
- copy encrypted backups to object storage outside the cluster
- test restore monthly
- attach the latest successful restore test to release operations review

## What Good Looks Like

A production-ready backup posture includes:

- scheduled automated backups
- successful backup logs retained
- at least one tested restore in the current release cycle
- documented ownership of backup and restore operations
- evidence attached to the production or release review

## Related Documents

Use this runbook with:

- [HA_GUIDE.md](HA_GUIDE.md)
- [PRODUCTION_CHECKLIST.md](PRODUCTION_CHECKLIST.md)
- [REFERENCE_DEPLOYMENT.md](REFERENCE_DEPLOYMENT.md)
