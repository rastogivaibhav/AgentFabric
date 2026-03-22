# Reference Deployment

This repo's reference deployment for the current product is:

- `api-gateway`
- `collector`
- `portal`
- external PostgreSQL
- external Redis

The reference path does not require `af-core`, Kafka, ClickHouse, or research/evaluation services.

## Recommended environments

### Local

Use:
- [docker-compose.yml](/C:/Users/vrast/Documents/Agentic%20Code/files/docker-compose.yml)
- [setup-local.sh](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/setup-local.sh)
- [bootstrap_local.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/bootstrap_local.ps1)

### Production-like Docker

Use:
- [docker-compose.yml](/C:/Users/vrast/Documents/Agentic%20Code/files/docker-compose.yml)
- [docker-compose.prod.yml](/C:/Users/vrast/Documents/Agentic%20Code/files/deploy/docker/docker-compose.prod.yml)

Required production inputs:
- `AF_JWT_SECRET`
- `AF_ADMIN_PASSWORD`
- `AF_VAULT_KEY`
- `AF_GATEWAY_AUTH_TOKEN`
- `DATABASE_URL`
- `REDIS_URL`
- `AF_CORS_ORIGINS`

### Kubernetes / Helm

Use:
- [agentfabric.yaml](/C:/Users/vrast/Documents/Agentic%20Code/files/deploy/k8s/agentfabric.yaml)
- [deploy/helm](/C:/Users/vrast/Documents/Agentic%20Code/files/deploy/helm)

For the current release scope, treat PostgreSQL and Redis as required backing services and keep provider support aligned to:
- `openai`
- `anthropic`
- `google`
- `vertexai`

## Operations Hardening

Production deployments should now include:

- Helm network isolation from [networkpolicies.yaml](/C:/Users/vrast/Documents/Agentic%20Code/files/deploy/helm/templates/networkpolicies.yaml)
- scheduled PostgreSQL backups from [backup-cronjob.yaml](/C:/Users/vrast/Documents/Agentic%20Code/files/deploy/helm/templates/backup-cronjob.yaml)
- documented backup and restore procedure in [BACKUP_RESTORE.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/BACKUP_RESTORE.md)
- HA review in [HA_GUIDE.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/HA_GUIDE.md)
- enterprise auth rollout in [SSO_RBAC_PLAN.md](/C:/Users/vrast/Documents/Agentic%20Code/files/docs/SSO_RBAC_PLAN.md)

Recommended production auth flags:

- `AF_SSO_REQUIRED=true` once OIDC is live
- `AF_PASSWORD_LOGIN_DISABLED=true` after SSO cutover

## Reference topology

```text
portal -> api-gateway -> postgres
                    -> redis
collector ---------> api-gateway
apps --------------> collector (OTLP)
apps --------------> api-gateway proxy / netproxy
```

This is a central deployment model. One shared control plane can serve many teams, environments, and onboarded workloads inside the same org or department.

## Coverage boundaries

The current product gives centralized visibility for:
- apps instrumented with the SDK
- services exporting OTLP spans
- LLM traffic routed through proxy or netproxy

It does not automatically monitor every host in an enterprise without onboarding those workloads into the telemetry or proxy path.

## Readiness endpoints

- gateway: `GET /healthz`, `GET /readyz`
- collector: `GET /healthz`, `GET /readyz`

## Minimum release validation

Run:
- health and readiness checks
- stack-health probe
- proxy-path proof
- portal build
- focused Go test suite
- release candidate validation script
- backup script dry run
- restore command rehearsal for the latest dump

Primary validation entrypoint:
- [run_release_candidate_validation.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/run_release_candidate_validation.ps1)
- [probe_stack_health.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/probe_stack_health.ps1)
- [probe-stack-health.sh](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/probe-stack-health.sh)
- [probe_proxy_path.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/probe_proxy_path.ps1)
- [probe-proxy-path.sh](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/probe-proxy-path.sh)

## Staging proof artifact

Treat the following outputs as release artifacts for every candidate deployment:

- gateway `GET /healthz`
- gateway `GET /readyz`
- collector `GET /healthz`
- collector `GET /readyz`
- stack-health probe output
- proxy-path probe output
- release candidate validation output
- latest backup job output
