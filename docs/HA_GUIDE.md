# High Availability Guide

## Baseline HA Shape

- 2+ `api-gateway` replicas
- 2+ `portal` replicas
- 1+ `collector` replica, scale horizontally for volume
- managed PostgreSQL with PITR enabled
- managed Redis with replication / failover
- external ingress or load balancer

## Kubernetes Controls

Use:

- pod disruption budgets from [runtime-stack.yaml](/C:/Users/vrast/Documents/Agentic%20Code/files/deploy/helm/templates/runtime-stack.yaml)
- network isolation from [networkpolicies.yaml](/C:/Users/vrast/Documents/Agentic%20Code/files/deploy/helm/templates/networkpolicies.yaml)
- readiness and liveness probes already defined in the chart

## Failure Domains

- `api-gateway` is stateless and can be replaced freely
- `portal` is stateless and can be replaced freely
- `collector` is mostly stateless; some in-flight telemetry can be retried
- PostgreSQL is the critical stateful dependency
- Redis affects performance and some runtime coordination, but not long-term evidence storage

## Upgrade Approach

1. back up PostgreSQL
2. apply migrations
3. roll `api-gateway`
4. roll `collector`
5. roll `portal`
6. run readiness, proxy, and GA gate checks

## Minimum Production Targets

- no single replica for gateway or portal
- database backups tested
- TLS termination defined
- OIDC / password-login policy agreed
- network policy enabled
