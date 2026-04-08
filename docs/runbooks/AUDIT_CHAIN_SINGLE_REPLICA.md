# Runbook: Audit Chain — Single-Replica Constraint

**Alert:** `AuditChainMultipleReplicas`
**Severity:** Critical
**Component:** af-core
**Affects:** Audit log tamper-evidence

---

## Overview

`af-core`'s `AuditWriter` maintains a hash chain over all audit log entries.
Each new entry is SHA-256 chained to the previous entry's hash, producing an
append-only, tamper-evident log.

**The current implementation stores `last_hash` in process memory
(`Mutex<String>`).** When more than one `af-core` replica is running:

- Each replica initialises its own chain from `"genesis"`
- Both chains grow independently with different entries and different hashes
- `GET /api/v1/audit/verify` reads mixed entries from both chains and sees
  hash mismatches — the chain appears broken even without tampering

This is a known architectural constraint tracked as
[roadmap item v1.2.0](#v120-roadmap).

---

## Detection

| Method | Signal |
|--------|--------|
| Prometheus alert | `AuditChainMultipleReplicas` fires when `kube_deployment_spec_replicas{deployment=~".*af-core.*"} > 1` |
| Audit verify endpoint | `GET /api/v1/audit/verify` returns `{"valid": false, "broken_at": ...}` |
| Helm values | `afCore.replicas > 1` or `afCore.hpa.enabled: true` with `minReplicas > 1` |

---

## Immediate Mitigation

### 1 — Scale af-core back to 1 replica

```bash
kubectl scale deployment govagn-af-core --replicas=1 -n govagn
```

Verify:
```bash
kubectl get pods -n govagn -l app.kubernetes.io/component=af-core
```

### 2 — Disable HPA if it re-scaled

```bash
kubectl patch hpa govagn-af-core -n govagn \
  -p '{"spec":{"minReplicas":1,"maxReplicas":1}}'
```

Or disable entirely:
```bash
kubectl delete hpa govagn-af-core -n govagn
```

### 3 — Verify audit chain health

```bash
curl -s -H "Authorization: Bearer $GV_TOKEN" \
  https://api.govagn.io/api/v1/audit/verify | jq .
```

Expected healthy response:
```json
{ "valid": true, "entries_checked": 1234 }
```

If the chain is broken, see [Chain Repair](#chain-repair-procedure) below.

### 4 — Confirm Helm values prevent recurrence

```bash
helm get values govagn -n govagn | grep -A 10 afCore
```

Ensure:
- `afCore.replicas: 1`
- `afCore.hpa.enabled: false`

If not, update:
```bash
helm upgrade govagn deploy/helm -n govagn \
  --set afCore.replicas=1 \
  --set afCore.hpa.enabled=false \
  --reuse-values
```

---

## Chain Repair Procedure

If the audit chain shows breaks due to multi-replica contamination:

1. **Do not delete records** — broken-chain entries are still valid audit data.

2. **Mark the break in the database** (manual SQL, requires DBA approval):
   ```sql
   -- Flag the first divergent entry with a sentinel note
   UPDATE audit_log
   SET chain_note = 'chain-break: multi-replica contamination YYYY-MM-DD'
   WHERE id = <first_divergent_id>;
   ```

3. **Re-seed the chain** from the last known-good entry:
   ```sql
   SELECT hash FROM audit_log ORDER BY created_at DESC LIMIT 1;
   ```
   Then restart af-core with `GV_AUDIT_SEED_HASH=<hash>` environment variable
   to re-initialise `last_hash` from the database value.

4. **Document the incident** in your compliance system with:
   - Start and end time of multi-replica window
   - Entry IDs affected
   - Root cause (how replicas were scaled up)
   - Remediation steps taken

---

## Root Cause Analysis Checklist

- [ ] Who/what scaled af-core above 1 replica?
  - Manual `kubectl scale`?
  - HPA firing on CPU threshold?
  - Helm upgrade with incorrect `afCore.replicas` value?
  - CI/CD pipeline deploying wrong values file?
- [ ] Was the Prometheus alert firing before the issue was detected?
  - If not: verify `kube-state-metrics` is running and Prometheus scrapes it
- [ ] How long was the chain diverged? (check audit log timestamps)

---

## v1.2.0 Roadmap

The fix is tracked in **AF-421: PostgreSQL-backed audit chain**.

The implementation will:
1. Replace `Mutex<String>` with a PostgreSQL advisory lock + `SELECT last_hash FROM audit_chain_state FOR UPDATE`
2. Use a single serialised transaction per entry: read lock → compute → write entry + update hash → commit
3. Enable `afCore.hpa.enabled: true` with safe `minReplicas: 2, maxReplicas: 8`
4. Add an integration test: spin up 3 af-core replicas under load and verify `GET /audit/verify` passes

Until v1.2.0 ships, **do not enable HPA for af-core in any environment**.

---

## Contacts

| Role | Contact |
|------|---------|
| On-call | PagerDuty — `govagn-platform` |
| af-core owner | Platform Engineering team |
| Compliance | Security team (for audit log incidents) |
