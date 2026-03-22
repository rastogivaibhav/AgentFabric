# Single-Tenant Installation

## When to Use This Model

Use the single-tenant model when:

- one organization, program, or business unit owns the platform
- one operations team manages the deployment
- tenant separation inside the control plane is not the primary design goal

This is the simplest production shape for AgentFabric.

## Target Topology

```text
portal
  -> api-gateway
collector
  -> api-gateway
api-gateway
  -> PostgreSQL
  -> Redis
applications
  -> collector and/or proxy and/or netproxy
```

## Required Components

- `api-gateway`
- `collector`
- `portal`
- PostgreSQL
- Redis

Recommended:
- ingress or reverse proxy
- TLS
- external secret management
- backup storage

## Deployment Options

### Option 1: Production-like Docker

Use:
- [docker-compose.yml](/C:/Users/vrast/Documents/Agentic%20Code/files/docker-compose.yml)
- [docker-compose.prod.yml](/C:/Users/vrast/Documents/Agentic%20Code/files/deploy/docker/docker-compose.prod.yml)

### Option 2: Helm or Kubernetes

Use:
- [values.yaml](/C:/Users/vrast/Documents/Agentic%20Code/files/deploy/helm/values.yaml)
- [runtime-stack.yaml](/C:/Users/vrast/Documents/Agentic%20Code/files/deploy/helm/templates/runtime-stack.yaml)

## Core Configuration

At minimum, configure:

- `AF_JWT_SECRET`
- `AF_ADMIN_PASSWORD`
- `AF_VAULT_KEY`
- `DATABASE_URL`
- `REDIS_URL`
- `AF_CORS_ORIGINS`
- `AF_GATEWAY_AUTH_TOKEN`

If using OIDC:
- `AF_OIDC_ISSUER`
- `AF_OIDC_CLIENT_ID`
- `AF_OIDC_CLIENT_SECRET`
- `AF_OIDC_REDIRECT_URI`

If enforcing SSO:
- `AF_SSO_REQUIRED=true`
- `AF_PASSWORD_LOGIN_DISABLED=true` after cutover

## Installation Sequence

1. Provision PostgreSQL and Redis.
2. Apply migrations.
3. Deploy the gateway.
4. Deploy the collector.
5. Deploy the portal.
6. Configure ingress and TLS.
7. Load pricing and policy defaults.
8. Create or confirm the first administrator.
9. Register the first provider key.
10. Run production probes and release validation.

## First-Day Operator Tasks

After the platform is live:

1. verify `/healthz` and `/readyz`
2. log in to the portal
3. confirm pricing rules are visible
4. confirm policy rules are visible
5. register one provider key
6. run one proxy request
7. verify one trace, one cost record, and one audit event

## Backup and Recovery

Single-tenant installations should still include:

- scheduled PostgreSQL backup
- restore procedure rehearsal
- retention settings

Use:
- [backup-cronjob.yaml](/C:/Users/vrast/Documents/Agentic%20Code/files/deploy/helm/templates/backup-cronjob.yaml)
- [BACKUP_RESTORE.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/BACKUP_RESTORE.md)
- [backup_postgres.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/backup_postgres.ps1)

## Validation Before Use

Run:

- [probe_stack_health.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/probe_stack_health.ps1)
- [probe_proxy_path.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/probe_proxy_path.ps1)
- [run_release_candidate_validation.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/run_release_candidate_validation.ps1)

And use:
- [PRODUCTION_CHECKLIST.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/PRODUCTION_CHECKLIST.md)

## Recommended Fit

This model is the right starting point when:

- one platform owner wants simplicity
- rollout speed matters more than tenant hierarchy
- the first goal is proving value, not maximizing tenant isolation
