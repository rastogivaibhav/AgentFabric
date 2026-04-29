# Secret Rotation Runbook

This document covers the procedures for rotating the master encryption key used to encrypt sensitive data in AgentFabric.

## Overview

AgentFabric uses AES-256-GCM encryption to protect sensitive fields like API keys, tokens, and credentials. The master key must be rotated regularly for compliance and security best practices.

**Rotation frequency:** Weekly (Sundays 2 AM UTC) via Kubernetes CronJob

**Key size:** 256 bits (32 bytes)

**Rotation method:** 
1. Generate new random master key
2. Query all encrypted secrets
3. Decrypt with old key
4. Re-encrypt with new key
5. Update database atomically
6. Audit record created in `secret_rotation_log`

---

## Prerequisites

- Access to `rotate-secrets` CLI tool (in Docker image)
- Database connection (via `DATABASE_URL` env var)
- Sufficient disk space for temporary decrypted data (during rotation only)
- No concurrent rotations (enforced by CronJob `concurrencyPolicy: Forbid`)

---

## Manual Rotation

### Step 1: Generate New Master Key

```bash
# Generate a random 256-bit key in hex format (32 bytes = 64 hex chars)
openssl rand -hex 32 > /tmp/new-master-key.txt

# Verify the key
wc -c /tmp/new-master-key.txt    # Should be 65 bytes (64 hex + newline)
cat /tmp/new-master-key.txt       # Should show 64 hex characters
```

### Step 2: Dry-Run the Rotation

Always test with dry-run first to verify the operation.

```bash
# For Docker Compose
docker-compose exec api-gateway rotate-secrets \
  -dry-run=true \
  -new-key=/tmp/new-master-key.txt \
  -user=admin@example.com

# For Kubernetes
kubectl run secret-rotation-test \
  --image=agentfabric:latest \
  --rm -it \
  -- rotate-secrets \
    -dry-run=true \
    -new-key=/tmp/new-master-key.txt \
    -user=admin@example.com
```

Expected output:
```
DRY-RUN: Simulating rotation
DRY-RUN: Would perform the following operations:
1. Query all encrypted secrets from database
2. Decrypt each secret with old master key
3. Re-encrypt each secret with new master key
4. Batch update database
5. Record rotation in secret_rotation_log
Run again with -dry-run=false to execute the rotation
```

### Step 3: Execute the Rotation

Once you've verified the dry-run output:

```bash
# For Docker Compose
docker-compose exec api-gateway rotate-secrets \
  -dry-run=false \
  -new-key=/tmp/new-master-key.txt \
  -user=admin@example.com

# For Kubernetes
kubectl run secret-rotation \
  --image=agentfabric:latest \
  --rm -it \
  -- rotate-secrets \
    -dry-run=false \
    -new-key=/tmp/new-master-key.txt \
    -user=admin@example.com
```

Expected output:
```
Starting secret key rotation
...
Secret rotation completed
Status: success
Items rotated: 24
Duration: 12 seconds
```

### Step 4: Verify the Rotation

```bash
# Check the audit log
psql $DATABASE_URL -c "
  SELECT id, rotated_at, items_rotated, status, duration_seconds
  FROM secret_rotation_log
  ORDER BY rotated_at DESC
  LIMIT 5;
"
```

Expected result:
```
 id | rotated_at | items_rotated | status  | duration_seconds
────┼────────────┼───────────────┼─────────┼──────────────────
  1 | 2024-01-10 |            24 | success |               12
(1 row)
```

---

## Automated Rotation (Kubernetes)

The automated rotation via CronJob:

1. **Runs every Sunday at 2 AM UTC** (configurable in `cronjob-secret-rotation.yaml`)
2. **Generates a new random key** in the `generate-key` init container
3. **Executes `rotate-secrets`** with the new key
4. **Records the result** in the audit table
5. **Alerts on failure** via Prometheus (if enabled)

### Monitor Automated Rotations

```bash
# View CronJob status
kubectl get cronjob secret-rotation -n default

# View recent job runs
kubectl get jobs -n default | grep secret-rotation

# Check latest job logs
LATEST_JOB=$(kubectl get jobs -n default --sort-by=.metadata.creationTimestamp | tail -1 | awk '{print $1}')
kubectl logs job/$LATEST_JOB -n default

# View last 10 rotation records
kubectl exec -it <api-gateway-pod> -- \
  psql $DATABASE_URL -c "
    SELECT id, rotated_at, items_rotated, status, error_message
    FROM secret_rotation_log
    ORDER BY rotated_at DESC
    LIMIT 10;
  "
```

### Check Rotation Schedule

```bash
# View the CronJob schedule (in UTC)
kubectl get cronjob secret-rotation -o jsonpath='{.spec.schedule}'

# Output: "0 2 * * 0" (2 AM every Sunday)
```

---

## Rollback Procedure

If a rotation fails and you need to revert:

### Option 1: Restore from Backup (Recommended)

```bash
# 1. Stop all services
docker-compose down

# 2. Restore database from backup
# (Use your backup tool: pg_dump, WAL-E, Velero, etc.)

# 3. Restart services
docker-compose up -d

# 4. Verify services are healthy
docker-compose ps
```

### Option 2: Manual Revert

If backups are unavailable:

```bash
# 1. Identify the failed rotation
psql $DATABASE_URL -c "
  SELECT id, rotated_at, status, error_message
  FROM secret_rotation_log
  WHERE status = 'failed'
  ORDER BY rotated_at DESC;
"

# 2. Examine the error
psql $DATABASE_URL -c "
  SELECT error_message FROM secret_rotation_log WHERE id = <failed_rotation_id>;
"

# 3. Contact support with:
#    - Rotation ID
#    - Error message
#    - Database state at time of failure
#    - Previous successful rotation ID
```

---

## Compliance & Auditing

### Audit Trail

All rotations are recorded in `secret_rotation_log` with:
- `key_id`: Which key was rotated (e.g., "master")
- `old_key_hash`: SHA-256 hash of old key (never the key itself)
- `new_key_hash`: SHA-256 hash of new key
- `items_rotated`: Count of secrets re-encrypted
- `status`: success, partial, or failed
- `encrypted_by_user`: Who performed the rotation
- `duration_seconds`: How long it took
- `error_message`: Any failures encountered

### Query the Rotation History

```sql
-- Last 10 rotations
SELECT
  id, rotated_at, items_rotated, status, duration_seconds
FROM secret_rotation_log
ORDER BY rotated_at DESC
LIMIT 10;

-- Rotations in the last 30 days
SELECT
  id, rotated_at, items_rotated, status
FROM secret_rotation_recent
ORDER BY rotated_at DESC;

-- Failed rotations
SELECT
  id, rotated_at, status, error_message
FROM secret_rotation_log
WHERE status = 'failed'
ORDER BY rotated_at DESC;
```

### Export Audit Report

```bash
# Export rotation audit as CSV
psql $DATABASE_URL -c "
  COPY (
    SELECT id, rotated_at, encrypted_by_user, items_rotated, status
    FROM secret_rotation_log
    WHERE rotated_at > NOW() - INTERVAL '1 year'
    ORDER BY rotated_at DESC
  ) TO STDOUT WITH CSV HEADER;
" > rotation-audit-$(date +%Y-%m-%d).csv
```

---

## Troubleshooting

### "Failed to decode key from hex"

**Problem:** The key file contains invalid hex characters.

**Solution:**
```bash
# Verify the key is valid hex
openssl rand -hex 32 > /tmp/new-key.txt
cat /tmp/new-key.txt | wc -c  # Should be 65 (64 hex + newline)
od -An -tx1 /tmp/new-key.txt   # View hex bytes
```

### "Key must be 32 bytes (256 bits)"

**Problem:** The key is the wrong size.

**Solution:**
```bash
# Generate exactly 32 bytes
openssl rand -hex 32 > /tmp/new-key.txt

# Verify size
echo -n "$(cat /tmp/new-key.txt | tr -d '\n')" | wc -c  # Should be 64
```

### "CronJob hasn't run recently"

**Problem:** The rotation job isn't executing on schedule.

**Solution:**
```bash
# Check if the CronJob exists
kubectl get cronjob secret-rotation -n default

# Check if there are suspended schedules
kubectl get cronjob secret-rotation -o jsonpath='{.spec.suspend}'

# Check recent job runs
kubectl get jobs -n default --sort-by=.metadata.creationTimestamp | grep secret-rotation | tail -5

# Check CronJob logs
kubectl describe cronjob secret-rotation -n default
```

### "Database connection refused"

**Problem:** `rotate-secrets` can't reach the database.

**Solution:**
```bash
# Verify DATABASE_URL is set correctly
echo $DATABASE_URL

# Test the connection
psql $DATABASE_URL -c "SELECT version();"

# For Kubernetes, check the secret
kubectl get secret agentfabric-db -o jsonpath='{.data.url}' | base64 -d
```

---

## Best Practices

1. **Rotate frequently:** Weekly is recommended, but adjust based on threat model
2. **Keep backups:** Always have a recent backup before rotating
3. **Test dry-run first:** Never skip the dry-run step
4. **Monitor rotations:** Set up Prometheus alerts for failures
5. **Audit thoroughly:** Review rotation audit logs regularly
6. **Secure key handling:** Never commit keys to Git or logs
7. **Use secure channels:** Only share keys via encrypted, out-of-band communication
8. **Gradual rollout:** Test rotation in staging before scheduling in production

---

## References

- [AES-256 Encryption](https://en.wikipedia.org/wiki/Advanced_Encryption_Standard)
- [Go crypto/aes](https://pkg.go.dev/crypto/aes)
- [Go crypto/cipher GCM](https://pkg.go.dev/crypto/cipher#NewGCM)
- [NIST Key Management Guidelines](https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-57pt1r5.pdf)
- [Kubernetes CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/)
