# Audit Log Verification & Hash-Chaining Guide

## Overview

AgentFabric implements a cryptographically secured audit trail using hash-chaining to ensure audit log integrity and immutability. This document explains how to verify audit chain integrity and validate log entries.

---

## Architecture

### Hash-Chaining Mechanism

Each audit log entry contains:

```sql
CREATE TABLE audit_log (
    id SERIAL PRIMARY KEY,
    timestamp TIMESTAMP DEFAULT NOW(),
    event_type VARCHAR(255),
    actor VARCHAR(255),
    resource VARCHAR(255),
    action VARCHAR(255),
    change_details JSONB,
    
    -- Hash chain fields
    hash VARCHAR(64),              -- SHA-256 hash of this entry
    previous_hash VARCHAR(64),     -- SHA-256 hash of previous entry
    verification_status VARCHAR(50) -- VALID, TAMPERED, BROKEN_CHAIN
);
```

### Hash Calculation

```
hash(entry_n) = SHA-256(
    entry_n.timestamp ||
    entry_n.event_type ||
    entry_n.actor ||
    entry_n.resource ||
    entry_n.action ||
    entry_n.change_details ||
    entry_(n-1).hash
)
```

If any field changes or the chain breaks, the hash will not match.

---

## Monitoring

### Verify Audit Chain Integrity (Daily)

```bash
# Check Prometheus metric
kubectl exec -it govagn-api-gateway-0 -n govagn -- \
  curl localhost:8080/metrics | grep govagn_audit_chain_valid

# Expected output:
# govagn_audit_chain_valid 1
```

**Alert if metric returns 0** (indicates tampering detected).

### Query Audit Log

```sql
-- Connect to PostgreSQL
kubectl exec -it postgres-0 -n govagn -- psql -U govagn

-- View recent audit entries
SELECT id, timestamp, event_type, actor, action, verification_status
FROM audit_log
ORDER BY id DESC
LIMIT 20;

-- Count by status
SELECT verification_status, COUNT(*) FROM audit_log GROUP BY verification_status;

-- Expected: All entries should be VALID
```

---

## Verification Procedures

### 1. Verify Hash Chain Integrity

```sql
-- Run this periodically to verify the entire chain
CREATE FUNCTION verify_audit_chain() RETURNS TABLE(
    entry_id BIGINT,
    is_valid BOOLEAN,
    error_reason TEXT
) AS $$
DECLARE
    current_entry audit_log%ROWTYPE;
    previous_entry audit_log%ROWTYPE;
    computed_hash VARCHAR(64);
BEGIN
    FOR current_entry IN
        SELECT * FROM audit_log ORDER BY id
    LOOP
        -- Compute expected hash
        computed_hash := encode(
            digest(
                COALESCE(current_entry.timestamp::TEXT, '') ||
                COALESCE(current_entry.event_type, '') ||
                COALESCE(current_entry.actor, '') ||
                COALESCE(current_entry.resource, '') ||
                COALESCE(current_entry.action, '') ||
                COALESCE(current_entry.change_details::TEXT, '') ||
                COALESCE(current_entry.previous_hash, ''),
                'sha256'
            ),
            'hex'
        );

        -- Check if hash matches
        IF computed_hash != current_entry.hash THEN
            RETURN QUERY SELECT
                current_entry.id,
                FALSE,
                'Hash mismatch: expected ' || computed_hash;
        ELSIF current_entry.id > 1 AND current_entry.previous_hash != LAG(current_entry.hash) OVER (ORDER BY id) THEN
            RETURN QUERY SELECT
                current_entry.id,
                FALSE,
                'Previous hash chain broken';
        ELSE
            RETURN QUERY SELECT
                current_entry.id,
                TRUE,
                NULL;
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Run verification
SELECT * FROM verify_audit_chain() WHERE NOT is_valid;
```

### 2. Export Audit Trail for External Verification

```bash
# Export audit log as CSV for offline verification
kubectl exec -it postgres-0 -n govagn -- psql -U govagn -c \
  "COPY audit_log TO STDOUT WITH CSV HEADER" > audit_log_export.csv

# Export as JSON for compliance
kubectl exec -it postgres-0 -n govagn -- psql -U govagn -c \
  "SELECT JSON_AGG(to_json(audit_log.*)) FROM audit_log" > audit_log_export.json
```

### 3. Verify Specific Audit Entry

```sql
-- Verify a specific entry by ID
SELECT
    id,
    timestamp,
    event_type,
    actor,
    hash,
    previous_hash,
    encode(
        digest(
            COALESCE(timestamp::TEXT, '') ||
            COALESCE(event_type, '') ||
            COALESCE(actor, '') ||
            COALESCE(resource, '') ||
            COALESCE(action, '') ||
            COALESCE(change_details::TEXT, '') ||
            COALESCE(previous_hash, ''),
            'sha256'
        ),
        'hex'
    ) AS computed_hash,
    hash = encode(
        digest(
            COALESCE(timestamp::TEXT, '') ||
            COALESCE(event_type, '') ||
            COALESCE(actor, '') ||
            COALESCE(resource, '') ||
            COALESCE(action, '') ||
            COALESCE(change_details::TEXT, '') ||
            COALESCE(previous_hash, ''),
            'sha256'
        ),
        'hex'
    ) AS is_valid
FROM audit_log
WHERE id = <ENTRY_ID>;
```

### 4. Check for Tampering Patterns

```sql
-- Find entries with mismatched hashes
SELECT id, event_type, actor, timestamp, verification_status
FROM audit_log
WHERE hash != CASE
    WHEN id = 1 THEN encode(digest(...), 'hex')  -- Compute for first entry
    ELSE NULL  -- PostgreSQL will compute based on previous
END;

-- Find broken chain sequences
SELECT
    a.id,
    a.event_type,
    a.hash,
    LAG(a.hash) OVER (ORDER BY a.id) as expected_previous_hash,
    a.previous_hash as actual_previous_hash,
    (LAG(a.hash) OVER (ORDER BY a.id) != a.previous_hash) as chain_broken
FROM audit_log a
WHERE (LAG(a.hash) OVER (ORDER BY a.id) != a.previous_hash)
   OR a.hash IS NULL;
```

---

## Secret Rotation Audit Trail

Secret rotation events are logged with full audit trail:

```sql
-- View all secret rotation events
SELECT
    id,
    timestamp,
    event_type,
    actor,
    action,
    change_details->>'secret_name' as secret_name,
    change_details->>'rotation_status' as rotation_status,
    verification_status
FROM audit_log
WHERE event_type = 'SECRET_ROTATION'
ORDER BY id DESC;

-- Count rotations by secret
SELECT
    change_details->>'secret_name' as secret_name,
    COUNT(*) as rotation_count,
    MAX(timestamp) as last_rotation
FROM audit_log
WHERE event_type = 'SECRET_ROTATION'
GROUP BY secret_name;
```

---

## Alerts & Monitoring

### Set Up Audit Chain Alert

```bash
# In Prometheus
cat > /etc/prometheus/rules/audit-alerts.yml << 'EOF'
groups:
  - name: audit
    rules:
    - alert: AuditChainBroken
      expr: govagn_audit_chain_valid == 0
      for: 1m
      annotations:
        summary: "Audit chain integrity compromised"
        description: "Audit log hash chain is broken. Immediate investigation required."
        severity: CRITICAL

    - alert: AuditLogTableGrowing
      expr: rate(govagn_audit_log_entries_total[1h]) > 1000
      for: 5m
      annotations:
        summary: "Unusual audit log growth"
        description: "Audit log entries growing at {{ $value }} entries/hour"
        severity: WARNING
EOF

# Reload Prometheus
kubectl rollout restart statefulset/prometheus -n govagn
```

---

## Compliance Reports

### Generate Audit Compliance Report

```bash
# SQL script to generate compliance report
cat > audit_compliance_report.sql << 'EOF'
-- Audit Compliance Report
SELECT
    'Total Audit Entries' as metric,
    COUNT(*)::TEXT as value
FROM audit_log
UNION ALL
SELECT
    'Valid Entries',
    COUNT(*)::TEXT
FROM audit_log
WHERE verification_status = 'VALID'
UNION ALL
SELECT
    'Tampered Entries',
    COUNT(*)::TEXT
FROM audit_log
WHERE verification_status != 'VALID'
UNION ALL
SELECT
    'Coverage Period',
    MIN(timestamp)::TEXT || ' to ' || MAX(timestamp)::TEXT
FROM audit_log
UNION ALL
SELECT
    'Last Entry Timestamp',
    MAX(timestamp)::TEXT
FROM audit_log
UNION ALL
SELECT
    'Chain Integrity Status',
    CASE WHEN COUNT(*) = SUM(CASE WHEN verification_status = 'VALID' THEN 1 ELSE 0 END)
         THEN 'INTACT'
         ELSE 'COMPROMISED'
    END
FROM audit_log;
EOF

# Run report
kubectl exec -it postgres-0 -n govagn -- psql -U govagn -f audit_compliance_report.sql
```

---

## Troubleshooting

### Chain Integrity Metric Shows 0

**Action:**
1. Immediately investigate database
2. Check recent writes
3. Review who has database access
4. Run full chain verification
5. Collect evidence for forensics
6. Page security team

### Missing Audit Entries

```sql
-- Check for gaps in sequence
SELECT id,
       LAG(id) OVER (ORDER BY id) as prev_id,
       (id - LAG(id) OVER (ORDER BY id) - 1) as gap_size
FROM audit_log
WHERE (id - LAG(id) OVER (ORDER BY id) - 1) > 0;
```

### Hash Mismatch on Specific Entry

```sql
-- Re-compute and compare
SELECT
    id,
    'Stored' as source,
    hash
FROM audit_log
WHERE id = <ENTRY_ID>
UNION ALL
SELECT
    <ENTRY_ID>,
    'Computed',
    encode(digest(...), 'hex')
```

---

## Best Practices

✅ **Run chain verification weekly** (at minimum)
✅ **Export audit logs monthly** for offline storage
✅ **Alert on chain integrity** issues immediately
✅ **Monitor audit log growth** for anomalies
✅ **Restrict database access** to minimize tampering risk
✅ **Maintain immutable backups** of audit logs
✅ **Document all verification runs** with timestamp and results
✅ **Report quarterly** to compliance/audit teams

---

## Implementation Status

- ✅ Hash-chaining algorithm implemented
- ✅ Verification functions available
- ✅ Prometheus metric `govagn_audit_chain_valid` exposed
- ✅ Secret rotation audit trail enabled
- ✅ PostgreSQL migration 0025 creates audit table
- ⚠️ Automated verification cron job (see OPERATIONAL-RUNBOOK.md)

---

**Last Updated:** 2026-05-04
**Maintained By:** Security & Compliance Team
