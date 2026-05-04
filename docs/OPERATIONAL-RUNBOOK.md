# AgentFabric Operational Runbook

## Overview
This guide provides procedures for deploying, monitoring, troubleshooting, and maintaining AgentFabric in production.

---

## 1. Deployment

### Prerequisites
- Kubernetes 1.24+
- Helm 3.10+
- PostgreSQL 14+
- Redis 7+
- ClickHouse 22+
- Kafka 3.0+

### Deployment Steps

```bash
# 1. Add Helm repository
helm repo add govagn https://helm.govagn.io
helm repo update

# 2. Create namespace
kubectl create namespace govagn

# 3. Create secrets for credentials
kubectl create secret generic govagn-secrets \
  --from-literal=postgres-password=<PASSWORD> \
  --from-literal=api-key=<API_KEY> \
  -n govagn

# 4. Deploy with Helm
helm install govagn govagn/govagn \
  --namespace govagn \
  --values values-production.yaml \
  --set monitoring.dashboards.enabled=true
```

### Post-Deployment Verification

```bash
# Check all pods are running
kubectl get pods -n govagn

# Verify services are accessible
kubectl port-forward -n govagn svc/govagn-api-gateway 8080:80

# Test OTLP endpoint
curl -X POST http://localhost:4317/v1/traces
```

---

## 2. Daily Monitoring

### Prometheus Alerts

Access Grafana dashboards at: `https://<cluster>/grafana`

**Critical Dashboards:**
- **Platform Overview**: Service health, request rate, error rate
- **Cost Report**: API spend tracking
- **Framework Breakdown**: Per-framework performance
- **Audit & Policy**: PII redactions, policy violations

### Key Metrics to Monitor

| Metric | Threshold | Action |
|--------|-----------|--------|
| API Gateway Error Rate | > 1% | Page on-call |
| Collector CPU | > 80% | Scale horizontally |
| Database Query Latency (P95) | > 500ms | Check slow query log |
| Audit Chain Validity | != 1 | Investigate immediately |

### Daily Checks

```bash
# Check all services are healthy
kubectl get pods -n govagn

# View recent errors
kubectl logs -n govagn -l app=govagn-api-gateway --tail=100 | grep -i error

# Check database connection pool
kubectl exec -it postgres-0 -n govagn -- psql -U govagn -c "SELECT count(*) FROM pg_stat_activity;"
```

---

## 3. Secret Rotation

AgentFabric includes automated secret rotation. Manual rotation if needed:

```bash
# Trigger secret rotation (CronJob runs daily at 2 AM UTC)
kubectl create job --from=cronjob/govagn-rotate-secrets secret-rotation-manual -n govagn

# Monitor rotation
kubectl logs -n govagn -l job-name=secret-rotation-manual -f

# Verify new secrets are active
kubectl get secret govagn-secrets -n govagn -o yaml | grep -A 5 "api-key"
```

---

## 4. Scaling

### Horizontal Scaling

```bash
# Scale API Gateway
kubectl scale deployment/govagn-api-gateway --replicas=5 -n govagn

# Scale Collector (stateless)
kubectl scale deployment/govagn-collector --replicas=3 -n govagn
```

### Vertical Scaling

Edit resource requests in `values-production.yaml`:

```yaml
api-gateway:
  resources:
    requests:
      memory: "2Gi"
      cpu: "1000m"
    limits:
      memory: "4Gi"
      cpu: "2000m"
```

---

## 5. Incident Response

### High Error Rate (>5%)

1. **Check Collector Health**
   ```bash
   kubectl logs -n govagn deployment/govagn-collector --tail=50
   ```

2. **Check API Gateway Health**
   ```bash
   kubectl logs -n govagn deployment/govagn-api-gateway --tail=50 | grep -i error
   ```

3. **Review Recent Changes**
   ```bash
   git log --oneline -20
   ```

4. **Rollback if Necessary**
   ```bash
   helm rollback govagn <REVISION> -n govagn
   ```

### Database Unavailable

1. **Check Pod Status**
   ```bash
   kubectl get pods -n govagn -l app=postgres
   ```

2. **Check Persistent Volume**
   ```bash
   kubectl get pvc -n govagn
   ```

3. **Verify Credentials**
   ```bash
   kubectl get secret govagn-secrets -n govagn -o yaml
   ```

### Memory Leak Suspected

1. **Monitor Memory Usage**
   ```bash
   kubectl top pods -n govagn
   ```

2. **Check Pod Restart Count**
   ```bash
   kubectl get pods -n govagn -o wide
   ```

3. **Collect Heap Dump** (Go services)
   ```bash
   kubectl exec -it govagn-api-gateway-0 -n govagn -- \
     curl localhost:6060/debug/pprof/heap > heap.dump
   ```

---

## 6. Backup & Recovery

### Daily Database Backup

```bash
# Backup PostgreSQL
pg_dump -h postgres.govagn.svc.cluster.local -U govagn govagn > /backups/govagn-$(date +%Y%m%d).sql

# Backup ClickHouse
clickhouse-client -h clickhouse.govagn.svc.cluster.local \
  --multiline --multiquery < /backups/govagn-export.sql
```

### Restore from Backup

```bash
# Restore PostgreSQL
psql -h postgres.govagn.svc.cluster.local -U govagn govagn < /backups/govagn-YYYYMMDD.sql

# Stop services during restore
kubectl scale deployment/govagn-api-gateway --replicas=0 -n govagn
```

---

## 7. Security

### mTLS Configuration (Production)

See `docs/MTLS-SETUP.md` for complete mTLS deployment guide.

### Audit Log Verification

Verify audit chain integrity daily:

```bash
# Check audit chain validity metric
kubectl exec -it govagn-api-gateway-0 -n govagn -- \
  curl localhost:8080/metrics | grep govagn_audit_chain_valid

# Should return: govagn_audit_chain_valid 1
```

### Rate Limiting

Check per-tenant rate limits:

```bash
# View current limits
kubectl get configmap govagn-rate-limits -n govagn -o yaml

# Update limits
kubectl patch configmap govagn-rate-limits -n govagn -p '{"data":{"per-user":"1000"}}'
```

---

## 8. Performance Tuning

### Database Optimization

```sql
-- Analyze query performance
ANALYZE;

-- Check index usage
SELECT schemaname, tablename, indexname FROM pg_indexes 
  WHERE schemaname NOT IN ('pg_catalog', 'information_schema');
```

### Cache Optimization

```bash
# Check Redis memory usage
kubectl exec -it redis-0 -n govagn -- redis-cli INFO memory

# Clear cache (if needed)
kubectl exec -it redis-0 -n govagn -- redis-cli FLUSHDB
```

---

## 9. Support & Escalation

**On-Call Contact:**
- PagerDuty: [govagn-oncall](https://pagerduty.com)
- Slack: #govagn-alerts

**Escalation Path:**
1. Alert fired → Check Grafana
2. > 15 min unresolved → Page on-call
3. > 30 min unresolved → Page lead engineer
4. > 1 hour unresolved → Page CTO

---

## 10. Version Upgrades

```bash
# Check current version
helm list -n govagn

# Upgrade to new version
helm upgrade govagn govagn/govagn \
  --namespace govagn \
  --values values-production.yaml

# Verify upgrade
kubectl rollout status deployment/govagn-api-gateway -n govagn
```

---

**Last Updated:** 2026-05-04
**Maintained By:** Infrastructure Team
