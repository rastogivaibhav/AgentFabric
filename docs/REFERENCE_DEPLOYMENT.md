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

## Reference topology

```text
portal -> api-gateway -> postgres
                    -> redis
collector -> api-gateway
clients -> proxy/netproxy -> api-gateway
```

## Minimum release validation

Run:
- health checks
- portal build
- focused Go test suite
- release candidate validation script

Primary validation entrypoint:
- [run_release_candidate_validation.ps1](/C:/Users/vrast/Documents/Agentic%20Code/files/scripts/run_release_candidate_validation.ps1)
