# AgentFabric — Kubernetes / Helm Installation Guide

This guide covers three deployment scenarios:

| Scenario | Time | When to use |
|---|---|---|
| [Quick Install](#quick-install) | 5 min | Cluster already set up, just want to run AgentFabric |
| [Production Install](#production-install) | 20 min | Full HA, TLS, secrets management, monitoring |
| [Air-Gapped Install](#air-gapped-install) | 30 min | No internet access, private registry |

---

## Prerequisites

| Tool | Version | Check |
|---|---|---|
| Kubernetes | 1.27+ | `kubectl version` |
| Helm | 3.12+ | `helm version` |
| `nginx-ingress` controller | any | `kubectl get ingressclass` |
| Persistent storage class | any | `kubectl get storageclass` |

Optional but recommended:
- `cert-manager` ≥ v1.14 for automatic TLS
- `kube-prometheus-stack` for Grafana dashboards

---

## Quick Install

Ideal for a demo, staging, or first look. Uses ClusterIP services, no TLS, in-cluster databases.

```bash
# 1. Add the Helm repository
helm repo add agentfabric https://charts.agentfabric.io
helm repo update

# 2. Create namespace
kubectl create namespace agentfabric

# 3. Install with minimum required overrides
helm install agentfabric agentfabric/agentfabric \
  --namespace agentfabric \
  --set global.environment=staging \
  --set postgresql.auth.password=changeme-pg \
  --set api.ingress.hosts[0].host=api.YOUR-DOMAIN.com \
  --set portal.ingress.hosts[0].host=app.YOUR-DOMAIN.com \
  --wait --timeout=10m
```

```bash
# 4. Check all pods are Running
kubectl get pods -n agentfabric

# Expected:
# agentfabric-collector-xxxxx       1/1     Running
# agentfabric-api-xxxxx             1/1     Running
# agentfabric-af-core-xxxxx         1/1     Running
# agentfabric-portal-xxxxx          1/1     Running
# agentfabric-postgresql-0          1/1     Running
# agentfabric-clickhouse-0          1/1     Running
# agentfabric-kafka-0               1/1     Running
# agentfabric-redis-master-0        1/1     Running
```

```bash
# 5. Open the portal
kubectl port-forward svc/agentfabric-portal 3000:80 -n agentfabric
# Visit http://localhost:3000 — login: admin / admin
```

---

## Production Install

### Step 1 — Create the namespace and secrets

```bash
kubectl create namespace agentfabric
```

Create a `secrets.yaml` (do **not** commit this file):

```yaml
# secrets.yaml — apply once, delete the file after
apiVersion: v1
kind: Secret
metadata:
  name: agentfabric-secrets
  namespace: agentfabric
type: Opaque
stringData:
  # Generate: openssl rand -hex 32
  jwt-secret: "REPLACE_WITH_64_CHAR_RANDOM_STRING"
  # Comma-separated for zero-downtime rotation: "new-secret,old-secret"
  jwt-secrets: "REPLACE_WITH_64_CHAR_RANDOM_STRING"
  admin-password: "REPLACE_WITH_STRONG_PASSWORD"
---
apiVersion: v1
kind: Secret
metadata:
  name: agentfabric-pg-secret
  namespace: agentfabric
type: Opaque
stringData:
  password: "REPLACE_WITH_STRONG_PG_PASSWORD"
```

```bash
kubectl apply -f secrets.yaml
rm secrets.yaml  # delete immediately after applying
```

### Step 2 — Create a production values override file

Save this as `values-prod.yaml`:

```yaml
global:
  environment: production
  image:
    registry: ghcr.io/agentfabric
    pullPolicy: IfNotPresent

# ─── TLS via cert-manager (Let's Encrypt) ────────────────────────────────────
certManager:
  enabled: true
  email: ops@your-org.com          # receives Let's Encrypt expiry notifications
  issuer:
    create: true
    name: agentfabric-letsencrypt
    kind: ClusterIssuer
    # Use staging for initial test, then switch to production:
    # server: https://acme-staging-v02.api.letsencrypt.org/directory
    server: https://acme-v02.api.letsencrypt.org/directory

# ─── API Gateway ─────────────────────────────────────────────────────────────
api:
  replicas: 3

  ingress:
    enabled: true
    className: nginx
    annotations:
      nginx.ingress.kubernetes.io/ssl-redirect: "true"
      cert-manager.io/cluster-issuer: agentfabric-letsencrypt
    hosts:
      - host: api.your-domain.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: agentfabric-api-tls
        hosts:
          - api.your-domain.com

  auth:
    # References the jwt-secrets key in agentfabric-secrets
    jwtSecrets: ""   # loaded from Secret in the chart

  cors:
    allowedOrigins:
      - "https://app.your-domain.com"

# ─── Portal ───────────────────────────────────────────────────────────────────
portal:
  replicas: 2

  ingress:
    enabled: true
    className: nginx
    annotations:
      nginx.ingress.kubernetes.io/ssl-redirect: "true"
      cert-manager.io/cluster-issuer: agentfabric-letsencrypt
    hosts:
      - host: app.your-domain.com
    tls:
      - secretName: agentfabric-portal-tls
        hosts:
          - app.your-domain.com

# ─── Collector DaemonSet ─────────────────────────────────────────────────────
collector:
  enabled: true
  tls:
    enabled: false   # Set true + provide certs for mTLS in high-security environments

# ─── af-core — single replica (audit chain constraint until v1.3) ────────────
afCore:
  replicas: 1       # DO NOT increase — see values.yaml comment on hash-chain constraint

# ─── PostgreSQL (Bitnami subchart) ───────────────────────────────────────────
postgresql:
  enabled: true
  auth:
    existingSecret: agentfabric-pg-secret  # uses password key from the secret
  primary:
    persistence:
      size: 100Gi
      storageClass: ""   # set to your fast SSD class (e.g. "gp3", "premium-rwo")

# ─── ClickHouse ───────────────────────────────────────────────────────────────
clickhouse:
  enabled: true
  persistence:
    size: 500Gi
    storageClass: ""

# ─── Kafka (Bitnami subchart) ─────────────────────────────────────────────────
kafka:
  enabled: true
  persistence:
    size: 50Gi

# ─── Redis ────────────────────────────────────────────────────────────────────
redis:
  enabled: true
  architecture: standalone

# ─── Monitoring (requires kube-prometheus-stack) ─────────────────────────────
monitoring:
  serviceMonitors:
    enabled: true
  dashboards:
    enabled: true
    grafanaNamespace: monitoring
```

### Step 3 — Install

```bash
helm repo add agentfabric https://charts.agentfabric.io
helm repo update

helm install agentfabric agentfabric/agentfabric \
  --namespace agentfabric \
  --values values-prod.yaml \
  --wait \
  --timeout=15m
```

### Step 4 — Verify

```bash
# All pods running
kubectl get pods -n agentfabric

# API gateway health
kubectl exec -n agentfabric deploy/agentfabric-api -- \
  wget -qO- http://localhost:8080/healthz
# → {"status":"ok"}

# af-core health (policy engine + ClickHouse writer)
kubectl exec -n agentfabric deploy/agentfabric-af-core -- \
  wget -qO- http://localhost:8889/health
# → {"status":"ok","service":"af-core"}

# Collector is receiving OTLP
kubectl logs -n agentfabric daemonset/agentfabric-collector --tail=20

# Send a test span
kubectl run test-span --rm -it --restart=Never \
  --image=curlimages/curl -- \
  curl -X POST http://agentfabric-collector.agentfabric:4318/v1/traces \
  -H 'Content-Type: application/json' \
  -d '{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"helm-test"}}]},"scopeSpans":[{"spans":[{"traceId":"aabbccddeeff00112233445566778899","spanId":"aabbccddaabbccdd","name":"test.span","startTimeUnixNano":"1700000000000000000","endTimeUnixNano":"1700000000500000000","attributes":[{"key":"gen_ai.system","value":{"stringValue":"crewai"}}]}]}]}]}'
```

Open `https://app.your-domain.com/traces` — the test span should appear.

---

## Upgrading

```bash
# Pull latest chart
helm repo update

# Dry-run to preview changes
helm upgrade agentfabric agentfabric/agentfabric \
  --namespace agentfabric \
  --values values-prod.yaml \
  --dry-run

# Apply upgrade (database migrations run automatically at startup)
helm upgrade agentfabric agentfabric/agentfabric \
  --namespace agentfabric \
  --values values-prod.yaml \
  --wait \
  --timeout=10m
```

### Zero-downtime JWT key rotation

```bash
# 1. Add the new secret alongside the old one
kubectl patch secret agentfabric-secrets -n agentfabric \
  --type=merge \
  -p '{"stringData":{"jwt-secrets":"NEW_SECRET,OLD_SECRET"}}'

# 2. Roll the api-gateway pods to pick up the new secret
kubectl rollout restart deployment/agentfabric-api -n agentfabric

# 3. After all clients have refreshed their tokens (wait 24h), remove the old secret
kubectl patch secret agentfabric-secrets -n agentfabric \
  --type=merge \
  -p '{"stringData":{"jwt-secrets":"NEW_SECRET"}}'
```

---

## Air-Gapped Install

For environments with no internet access (on-premise, classified networks).

### Step 1 — Pull and push images to your private registry

```bash
REGISTRY=registry.your-org.internal
VERSION=1.2.0

IMAGES=(
  "ghcr.io/agentfabric/collector:${VERSION}"
  "ghcr.io/agentfabric/api-gateway:${VERSION}"
  "ghcr.io/agentfabric/af-core:${VERSION}"
  "ghcr.io/agentfabric/portal:${VERSION}"
)

for img in "${IMAGES[@]}"; do
  name=$(echo $img | cut -d/ -f3)
  docker pull $img
  docker tag $img ${REGISTRY}/agentfabric/${name}
  docker push ${REGISTRY}/agentfabric/${name}
done
```

### Step 2 — Package the Helm chart

```bash
# On a machine with internet access:
helm repo add agentfabric https://charts.agentfabric.io
helm pull agentfabric/agentfabric --version 1.2.0
# Creates: agentfabric-1.2.0.tgz

# Transfer agentfabric-1.2.0.tgz to air-gapped environment
```

### Step 3 — Install from local chart

```yaml
# Add to values-prod.yaml:
global:
  image:
    registry: registry.your-org.internal
    pullPolicy: Always   # force re-pull from private registry
```

```bash
helm install agentfabric ./agentfabric-1.2.0.tgz \
  --namespace agentfabric \
  --create-namespace \
  --values values-prod.yaml \
  --wait
```

---

## Uninstall

```bash
# Remove the release (keeps PVCs and secrets by default)
helm uninstall agentfabric -n agentfabric

# To also delete persistent data (IRREVERSIBLE):
kubectl delete pvc -n agentfabric --all
kubectl delete secret -n agentfabric agentfabric-secrets agentfabric-pg-secret
kubectl delete namespace agentfabric
```

---

## Troubleshooting

### af-core is CrashLoopBackOff

```bash
kubectl logs -n agentfabric deploy/agentfabric-af-core --previous
```

Common causes:
- Kafka not yet healthy — af-core retries connections, but may crash before Kafka is ready. Solution: `kubectl rollout restart deployment/agentfabric-af-core -n agentfabric` after Kafka is Running.
- ClickHouse unreachable — check `kubectl get pods -n agentfabric | grep clickhouse`.

### api-gateway not writing to Kafka

```bash
# Check KAFKA_BROKERS is set
kubectl exec -n agentfabric deploy/agentfabric-api -- env | grep KAFKA
# Should show: KAFKA_BROKERS=agentfabric-kafka:9092

# Check for warning logs
kubectl logs -n agentfabric deploy/agentfabric-api | grep -i kafka
```

### ClickHouse is empty

af-core writes to ClickHouse after consuming from Kafka. Verify the consumer is running:

```bash
kubectl exec -n agentfabric deploy/agentfabric-af-core -- \
  wget -qO- http://localhost:8889/metrics | grep afcore_spans_consumed
# afcore_spans_consumed_total 42  ← should be non-zero after spans are sent
```

### Database migration failed at startup

```bash
kubectl logs -n agentfabric deploy/agentfabric-api | grep -i migrat
```

Run migrations manually if needed:

```bash
kubectl exec -n agentfabric deploy/agentfabric-api -- \
  /agentfabric-gateway -migrate-only
```

### Portal shows "No data"

The portal reads from the api-gateway. Check:
1. `kubectl logs deploy/agentfabric-api -n agentfabric` for errors
2. Verify spans are actually being sent: `kubectl logs daemonset/agentfabric-collector -n agentfabric | grep processed`
3. Verify `VITE_API_URL` is set correctly in the portal deployment

---

## Configuration Reference

Key values with their defaults and descriptions:

| Value | Default | Description |
|---|---|---|
| `global.environment` | `production` | Sets log verbosity and dev-mode flags |
| `collector.tls.enabled` | `true` | Enable mTLS between collector and api-gateway |
| `afCore.replicas` | `1` | Do not increase until v1.3 (audit chain constraint) |
| `api.replicas` | `3` | api-gateway replicas (stateless, safe to scale) |
| `portal.replicas` | `2` | Portal replicas (stateless) |
| `certManager.enabled` | `false` | Auto-provision TLS via Let's Encrypt |
| `postgresql.primary.persistence.size` | `100Gi` | PostgreSQL data volume |
| `clickhouse.persistence.size` | `500Gi` | ClickHouse data volume (spans: 90-day TTL) |
| `monitoring.serviceMonitors.enabled` | `false` | Prometheus ServiceMonitor resources |
| `monitoring.dashboards.enabled` | `false` | Grafana dashboard ConfigMaps |

Full reference: [`deploy/helm/values.yaml`](deploy/helm/values.yaml)
