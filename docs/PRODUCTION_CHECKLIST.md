# AgentFabric — Production Readiness Checklist

**Version**: 1.1.0
**Date**: 2026-03-16
**Audience**: Ops team, on-call engineers, release managers
**Purpose**: Pre-deploy gate, day-2 operations reference, and tribal-knowledge replacement.

Tick every box before merging to `main` and before every production deployment.
Any unchecked box is a deployment blocker unless a named engineer signs off with justification.

---

## 1. Pre-Deploy Gate

### 1.1 Code Quality
- [ ] `make build` passes on CI (Go binary + portal bundle — zero compile errors)
- [ ] `make test` passes (all Go unit tests + 113 portal vitest tests)
- [ ] `make lint` passes (`golangci-lint` + `npx tsc --noEmit` + ESLint — zero warnings)
- [ ] No `TODO(prod)` or `FIXME` comments introduced since last release
- [ ] PR has ≥ 1 peer review approval from a team member other than the author

### 1.2 Security
- [ ] `AF_ADMIN_PASSWORD` is **not** `admin` — verify with `echo $AF_ADMIN_PASSWORD | wc -c` (≥ 32 chars)
- [ ] `AF_JWT_SECRET` is **not** `dev-secret-change-in-production` — minimum 32 random bytes
- [ ] `POSTGRES_PASSWORD`, `REDIS_URL` passwords are not default values
- [ ] `.env` file is **not** committed — `git log --all -- .env` returns empty
- [ ] `VITE_SHOW_DEFAULT_CREDS` is **not** `true` in production build args
- [ ] `AF_AUTH_DISABLED` is absent or `false` in production environment
- [ ] TLS decision documented: either `AF_TLS_ENABLED=true` with valid cert paths, or TLS terminated at load balancer (document which)
- [ ] Secrets stored in a secrets manager (Vault, AWS Secrets Manager, K8s Secrets with external-secrets-operator) — **never** in repo or plain-text config files

### 1.3 Infrastructure
- [ ] K8s: `kubectl apply --dry-run=client -f deploy/k8s/agentfabric.yaml` — zero errors
- [ ] K8s: all 3 PodDisruptionBudgets present — `kubectl get pdb -n agentfabric` shows `agentfabric-gateway-pdb`, `agentfabric-collector-pdb`, `agentfabric-af-core-pdb`
- [ ] K8s: HPA enabled for gateway and af-core — `kubectl get hpa -n agentfabric`
- [ ] K8s: `imagePullPolicy: IfNotPresent` replaced with `Always` for non-SHA image tags, OR image tags are pinned to SHA digests
- [ ] Helm: `helm lint deploy/helm` passes; `helm template` produces ≥ 10 resources

### 1.4 Database
- [ ] `make migrate/status DATABASE_URL=<prod_dsn>` shows expected version (currently: 1)
- [ ] `make migrate/up DATABASE_URL=<prod_dsn>` has been run against a **staging** environment first
- [ ] Database backup verified < 24 hours old before deploying schema migrations
- [ ] `af_audit_writer` role password has been changed from `CHANGE_IN_PRODUCTION` via `ALTER ROLE af_audit_writer PASSWORD '<strong_secret>'`

### 1.5 Observability
- [ ] Prometheus is scraping both collector (:9090) and api-gateway (:8080/metrics) — verify in Prometheus targets UI
- [ ] Grafana dashboard `agentfabric-overview.json` loads without errors
- [ ] At least one alert rule is in `INACTIVE` (not `ERROR`) state — confirms Prometheus rules are loaded
- [ ] Log aggregation (e.g. Loki, CloudWatch) is collecting from all pods

---

## 2. Secrets Management

### 2.1 Required Secrets Reference

| Secret | Env Var | Min Length | Generate With |
|--------|---------|-----------|---------------|
| JWT signing key | `AF_JWT_SECRET` | 32 chars | `openssl rand -base64 32` |
| Admin password | `AF_ADMIN_PASSWORD` | 20 chars | `openssl rand -base64 24` |
| PostgreSQL password | `POSTGRES_PASSWORD` / in `DATABASE_URL` | 24 chars | `openssl rand -base64 24` |
| Redis password | in `REDIS_URL` | 20 chars | `openssl rand -base64 20` |
| ClickHouse password | in `CLICKHOUSE_URL` | 20 chars | `openssl rand -base64 20` |
| Audit writer role | `af_audit_writer` DB role | 20 chars | `openssl rand -base64 20` |
| Grafana admin | `GF_ADMIN_PASSWORD` | 16 chars | `openssl rand -base64 16` |

### 2.2 JWT Key Rotation (Zero-Downtime)

Rotate the JWT signing key without invalidating active sessions:

```bash
# 1. Generate new key
NEW_SECRET=$(openssl rand -base64 32)

# 2. Update AF_JWT_SECRETS to: "new_secret,old_secret"
#    AF_JWT_SECRETS accepts comma-separated keys.
#    The FIRST key is used for signing; all keys are accepted for verification.
kubectl patch secret agentfabric-secrets -n agentfabric \
  --type='json' \
  -p='[{"op":"replace","path":"/data/jwt-secrets","value":"'"$(echo -n "${NEW_SECRET},${OLD_SECRET}" | base64 -w0)"'"}]'

# 3. Rolling-restart the gateway to pick up the new secret
kubectl rollout restart deployment/agentfabric-gateway -n agentfabric
kubectl rollout status deployment/agentfabric-gateway -n agentfabric

# 4. Wait for existing tokens to expire (default: 8 hours / AF_SESSION_MAX_AGE)
#    Then remove the old key from AF_JWT_SECRETS:
kubectl patch secret agentfabric-secrets -n agentfabric \
  --type='json' \
  -p='[{"op":"replace","path":"/data/jwt-secrets","value":"'"$(echo -n "${NEW_SECRET}" | base64 -w0)"'"}]'

# 5. Update AF_JWT_SECRET to the new key (collector still uses AF_JWT_SECRET)
kubectl patch secret agentfabric-secrets -n agentfabric \
  --type='json' \
  -p='[{"op":"replace","path":"/data/jwt-secret","value":"'"$(echo -n "${NEW_SECRET}" | base64 -w0)"'"}]'

# 6. Restart collector DaemonSet
kubectl rollout restart daemonset/agentfabric-collector -n agentfabric
```

### 2.3 Admin Password Rotation

```bash
NEW_PASS=$(openssl rand -base64 24)

# Update the K8s secret
kubectl patch secret agentfabric-secrets -n agentfabric \
  --type='json' \
  -p='[{"op":"replace","path":"/data/admin-password","value":"'"$(echo -n "${NEW_PASS}" | base64 -w0)"'"}]'

# Update the users table (the bcrypt hash stored at login time must also be updated)
psql $DATABASE_URL -c \
  "UPDATE users SET password_hash = crypt('${NEW_PASS}', gen_salt('bf', 10)), updated_at = NOW() WHERE username = 'admin';"

# Restart gateway to pick up new env var
kubectl rollout restart deployment/agentfabric-gateway -n agentfabric
```

---

## 3. TLS / Certificate Operations

### 3.1 Dev Self-Signed Certs (local only)

```bash
make certs
# Certs land in deploy/certs/ — gitignored
# Set in environment:
#   AF_TLS_ENABLED=true
#   AF_TLS_CERT_FILE=deploy/certs/server.crt
#   AF_TLS_KEY_FILE=deploy/certs/server.key
```

### 3.2 Production Certificate Renewal

**Before cert expires** (set a calendar reminder for 30 days before expiry):

```bash
# 1. Check current expiry
openssl x509 -enddate -noout -in /path/to/server.crt

# 2. Obtain renewed cert from your CA (Let's Encrypt, internal PKI, etc.)

# 3. Update the K8s TLS secret
kubectl create secret tls agentfabric-tls \
  --cert=/path/to/new.crt \
  --key=/path/to/new.key \
  -n agentfabric \
  --dry-run=client -o yaml | kubectl apply -f -

# 4. Verify the gateway pod picks up the new cert (no restart required if secret is mounted as volume)
# If using env vars, restart the deployment:
kubectl rollout restart deployment/agentfabric-gateway -n agentfabric

# 5. Verify new expiry
kubectl exec -n agentfabric deployment/agentfabric-gateway -- \
  openssl s_client -connect localhost:8080 </dev/null 2>/dev/null | openssl x509 -noout -enddate
```

### 3.3 mTLS Collector Configuration

The collector supports mTLS when `AF_TLS_ENABLED=true`. To require client certificates:

```yaml
# collector.yaml (ConfigMap)
auth:
  require_auth: true  # JWT bearer on OTLP ingest
# mTLS is handled at the TLS layer — provide CA cert to verify client certs
```

---

## 4. Deployment Runbook

### 4.1 Standard Rolling Deploy (K8s)

```bash
# 1. Pre-flight
kubectl get nodes                          # all nodes Ready
kubectl get pdb -n agentfabric             # all PDBs present

# 2. Apply migrations (runs on startup automatically, but verify manually first)
make migrate/status DATABASE_URL=$DATABASE_URL   # note current version
make migrate/up    DATABASE_URL=$DATABASE_URL     # apply pending

# 3. Apply manifests
kubectl apply -f deploy/k8s/agentfabric.yaml

# 4. Monitor rollout
kubectl rollout status deployment/agentfabric-gateway -n agentfabric
kubectl rollout status deployment/agentfabric-af-core -n agentfabric
kubectl rollout status deployment/agentfabric-portal  -n agentfabric
kubectl rollout status daemonset/agentfabric-collector -n agentfabric

# 5. Smoke test
curl -sf https://<gateway_host>/healthz | jq .
```

### 4.2 Rollback Procedure

**Trigger**: any of — error rate > 1% over 5 minutes, P99 latency > 2s, health endpoint returning non-200, or on-call engineer judgement.

```bash
# Immediate: roll back gateway (most common failure point)
kubectl rollout undo deployment/agentfabric-gateway -n agentfabric
kubectl rollout status deployment/agentfabric-gateway -n agentfabric

# If schema migration was applied and must be reversed:
make migrate/down DATABASE_URL=$DATABASE_URL    # rolls back ONE step
make migrate/status DATABASE_URL=$DATABASE_URL  # confirm version

# Full rollback to previous image (replace <previous_tag>):
kubectl set image deployment/agentfabric-gateway \
  gateway=ghcr.io/agentfabric/api-gateway:<previous_tag> \
  -n agentfabric

# Verify
curl -sf https://<gateway_host>/healthz
kubectl get pods -n agentfabric -w
```

**Decision tree for rollback**:
1. Panic / OOM in logs → roll back image immediately
2. DB error after migration → `make migrate/down` then roll back image
3. Auth failures (401 storm) → check `AF_JWT_SECRET` matches between collector + gateway
4. Rate limit flood (429 storm) → temporarily increase `AF_RATE_LIMIT_RPM` or disable per-tenant limiter

### 4.3 Docker Compose Production Deploy

```bash
# Required env vars — set before starting
export AF_JWT_SECRET=$(openssl rand -base64 32)
export AF_ADMIN_PASSWORD=$(openssl rand -base64 24)
export AF_CORS_ORIGINS="https://app.yourdomain.com"
export DATABASE_URL="postgres://fabric:<pass>@postgres:5432/agentfabric?sslmode=require"
export REDIS_URL="redis://:<pass>@redis:6379"
export POSTGRES_PASSWORD="<pass>"
export VITE_API_URL="https://api.yourdomain.com"
export VITE_WS_URL="wss://api.yourdomain.com"

# Deploy
make prod-up

# Verify
docker compose ps
docker compose logs api-gateway --tail 20
curl -sf http://localhost:8080/healthz
```

---

## 5. Service Level Objectives (SLOs)

These are the contractual targets. Breaching them triggers the incident response procedure (§6).

| SLO | Target | Measurement Window | Alert Threshold |
|-----|--------|--------------------|-----------------|
| API Gateway availability | 99.9% | 30-day rolling | < 99.5% over 1 hour |
| API P99 latency (`/api/v1/*`) | < 300 ms | 5-minute window | > 500 ms for 5 min |
| API P50 latency | < 50 ms | 5-minute window | > 100 ms for 10 min |
| Span ingest throughput | > 10,000 spans/s per node | 1-minute window | < 5,000 for 3 min |
| Processor queue depth | < 50,000 spans queued | 1-minute window | > 200,000 for 2 min |
| Error rate (5xx) | < 0.1% of requests | 5-minute window | > 1% for 5 min |
| PII redaction coverage | 100% (zero leakage) | Per-span | Any span with PII flag=false but PII pattern detected |

**Error budget**: 99.9% availability = 43.8 minutes downtime per 30-day period.

**SLO burn-rate alerts** (Prometheus alert rules in `monitoring/alerts.yml`):
- `HighErrorRate`: > 5% error rate for 5 min → PagerDuty P2
- `HighP95Latency`: P95 > 2s for 10 min → PagerDuty P2
- `ServiceDown`: any service UP gauge = 0 → PagerDuty P1
- `CollectorNotReceivingSpans`: 0 spans/min for 5 min → PagerDuty P1
- `HighProcessorQueueDepth`: queue > 500k for 5 min → PagerDuty P2
- `UnexpectedCostSpike`: projected > $100/hour → PagerDuty P3

---

## 6. On-Call Contacts

> **Action required**: Fill in this section before go-live. Template provided.

| Role | Name | Primary Contact | Escalation Contact |
|------|------|----------------|--------------------|
| On-call engineer (primary) | _TBD_ | _phone/Slack_ | _TBD_ |
| On-call engineer (secondary) | _TBD_ | _phone/Slack_ | _TBD_ |
| Engineering lead | _TBD_ | _phone/Slack_ | _TBD_ |
| Database admin (DBA) | _TBD_ | _phone/Slack_ | _TBD_ |
| Security officer | _TBD_ | _phone/Slack_ | _TBD_ |
| Customer success (for tenant impact) | _TBD_ | _email/Slack_ | _TBD_ |

**Escalation SLAs**:
- P1 (service down): respond within 5 min, escalate after 15 min
- P2 (degraded performance): respond within 15 min, escalate after 1 hour
- P3 (non-critical): respond within 4 hours, next business day acceptable

**Runbook location**: `docs/PRODUCTION_CHECKLIST.md` (this file)
**Status page**: _TBD_
**Incident channel**: _TBD (e.g. #incidents-agentfabric on Slack)_

---

## 7. Post-Deploy Verification

Run after every production deployment, regardless of scope:

```bash
# 1. Health checks
curl -sf https://<gateway_host>/healthz | jq .
curl -sf https://<portal_host>/          | grep -q "AgentFabric"

# 2. Auth smoke test (replace creds)
TOKEN=$(curl -sf -X POST https://<gateway_host>/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"<admin_user>","password":"<admin_pass>"}' | jq -r .token)
echo "Token obtained: ${TOKEN:0:20}..."

# 3. Authenticated API call
curl -sf https://<gateway_host>/api/v1/environments \
  -H "Authorization: Bearer $TOKEN" | jq length

# 4. Migration version matches expected
make migrate/status DATABASE_URL=$DATABASE_URL

# 5. PDB still in place (cluster resilience)
kubectl get pdb -n agentfabric

# 6. No pods in CrashLoopBackOff
kubectl get pods -n agentfabric | grep -v Running | grep -v Completed
```

All six commands must succeed with expected output. Any failure is a rollback trigger.

---

## 8. Known Limitations (v1.1.0)

These are accepted risks tracked for future sprints. Each item has an owner and target version.

| Item | Risk Level | Target Version | Owner |
|------|-----------|---------------|-------|
| `af_audit_writer` bcrypt password set to `CHANGE_IN_PRODUCTION` in init SQL — must be changed manually after first migrate | High | v1.2.0 (automate via migration) | Backend |
| mTLS between collector pods not enabled by default in docker-compose | Medium | v1.2.0 | Platform |
| Portal edit-user modal links to `/users/:id/edit` which is not yet implemented | Low | v1.2.0 | Frontend |
| Grafana alert PagerDuty/Slack notification channels not configured | Medium | v1.2.0 | Ops |
| No automated certificate rotation (cert-manager not yet configured) | Medium | v1.2.0 | Platform |
| Per-tenant audit retention policy not enforced (data grows unbounded) | Low | v1.3.0 | Backend |
