# Runbook: `af_audit_writer` Password Rotation

**Audience:** Platform / SRE on-call
**Frequency:** Rotate every 90 days or immediately after any suspected credential exposure
**SOC 2 Control:** CC6.1 — Logical access to production systems uses unique credentials that are rotated on schedule
**Related migration:** `deploy/migrations/001_initial_schema.up.sql` (lines 142–149)

---

## Background

`af_audit_writer` is a PostgreSQL role with **INSERT-only** access to `policy_audit_log`.
It is used exclusively by af-core to append hash-chained audit entries — it cannot SELECT, UPDATE, or DELETE any row.

The migration creates the role as `NOLOGIN` by default. Before af-core can connect as this role, an operator must:
1. Enable `LOGIN` on the role
2. Set a strong random password
3. Store the password in the `agentfabric-secrets` Kubernetes Secret (or equivalent vault entry)
4. Set `AF_AUDIT_DSN` in the af-core deployment to a connection string using this role

This runbook covers the initial setup **and** periodic rotation.

---

## Pre-requisites

- `psql` access to the production PostgreSQL instance (or `kubectl exec` into the postgres pod)
- Access to the cluster secret store (`kubectl` with write to `agentfabric-secrets`, or Vault operator access)
- `openssl` or `pwgen` for generating random passwords

---

## Step 1 — Generate a new password

```bash
# Generate a 32-character random password (no special chars that confuse psql DSN)
NEW_PASS=$(openssl rand -base64 32 | tr -d '/+=' | head -c 32)
echo "New password: ${NEW_PASS}"
# Store it somewhere safe — you'll need it in steps 2 and 3
```

---

## Step 2 — Apply the new password to PostgreSQL

Connect to the production database as a superuser (e.g. `fabric` / the `postgres` role):

```bash
# Kubernetes — exec into the postgres pod
kubectl exec -n agentfabric -it \
  $(kubectl get pod -n agentfabric -l app=postgresql -o jsonpath='{.items[0].metadata.name}') \
  -- psql -U fabric -d agentfabric
```

```sql
-- Enable LOGIN and set the new password atomically.
-- First rotation: role was created NOLOGIN by migration — add LOGIN here.
ALTER ROLE af_audit_writer WITH LOGIN PASSWORD '<NEW_PASS>';

-- Verify
\du af_audit_writer
-- Expected output includes: Login = yes, Superuser = no, Create DB = no
```

> **No downtime required.** af-core will transparently reconnect using the new DSN once the
> Secret is updated (step 3) and the pod is rolled (step 4).

---

## Step 3 — Update the Kubernetes Secret

```bash
# Base64-encode the connection string
AF_AUDIT_DSN="postgres://af_audit_writer:<NEW_PASS>@postgres:5432/agentfabric?sslmode=require"
ENCODED=$(echo -n "${AF_AUDIT_DSN}" | base64 -w0)

# Patch the secret
kubectl patch secret agentfabric-secrets -n agentfabric \
  --type='json' \
  -p="[{\"op\": \"replace\", \"path\": \"/data/af-audit-dsn\", \"value\": \"${ENCODED}\"}]"

# Verify the secret was updated
kubectl get secret agentfabric-secrets -n agentfabric \
  -o jsonpath='{.data.af-audit-dsn}' | base64 -d
```

For **Vault** deployments: update the `secret/agentfabric/production/af-audit-dsn` path.

---

## Step 4 — Roll af-core to pick up the new Secret

```bash
kubectl rollout restart deployment/af-core -n agentfabric
kubectl rollout status deployment/af-core -n agentfabric --timeout=120s
```

af-core reads `AF_AUDIT_DSN` from the Secret at pod startup via `envFrom.secretRef`.
The rolling update ensures zero downtime: new pods come up with the new password before old pods terminate.

---

## Step 5 — Verify audit writes are working

```bash
# Check af-core logs for successful audit connection
kubectl logs -n agentfabric -l app=af-core --tail=50 | grep -i "audit"
# Expected: "af_audit_writer connected" or similar startup log

# Confirm no auth errors in PostgreSQL logs
kubectl logs -n agentfabric -l app=postgresql --tail=50 | grep "af_audit_writer"
# Must NOT contain: "password authentication failed"

# Spot-check a recent audit entry was written
kubectl exec -n agentfabric -it \
  $(kubectl get pod -n agentfabric -l app=postgresql -o jsonpath='{.items[0].metadata.name}') \
  -- psql -U fabric -d agentfabric \
  -c "SELECT id, evaluated_at, result FROM policy_audit_log ORDER BY id DESC LIMIT 5;"
```

---

## Step 6 — Record the rotation

Update the rotation log in your internal wiki / SOC 2 evidence tracker:

| Date | Rotated by | Ticket | New expiry |
|------|------------|--------|------------|
| YYYY-MM-DD | `<your name>` | `OPS-NNN` | YYYY-MM-DD (+90 days) |

Schedule the next rotation reminder (90 days) in your alerting / task system.

---

## Rollback

If af-core fails to start after the rotation (e.g. wrong password applied):

```bash
# 1. Restore the old password in PostgreSQL
ALTER ROLE af_audit_writer WITH LOGIN PASSWORD '<OLD_PASS>';

# 2. Restore the old Secret value
kubectl patch secret agentfabric-secrets -n agentfabric \
  --type='json' \
  -p="[{\"op\": \"replace\", \"path\": \"/data/af-audit-dsn\", \"value\": \"<OLD_BASE64>\"}]"

# 3. Roll af-core back
kubectl rollout undo deployment/af-core -n agentfabric
```

---

## Least-privilege reference

The `af_audit_writer` role holds exactly:

```sql
GRANT INSERT ON policy_audit_log TO af_audit_writer;
GRANT USAGE ON SEQUENCE policy_audit_log_id_seq TO af_audit_writer;
-- No SELECT, UPDATE, DELETE granted (enforced by DB rules + GRANT table)
```

Database-level `NO UPDATE` and `NO DELETE` rules on `policy_audit_log` are independent of the role —
even a superuser running as `af_audit_writer` cannot update or delete audit rows.
