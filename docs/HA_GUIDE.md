# High Availability Guide

## Purpose

This guide describes the minimum high-availability posture for running Govagn as a serious production candidate.

Use it to answer:

- what should be replicated
- which components are stateless
- where the critical failure domains are
- how upgrades should be performed safely

## Baseline HA Shape

Recommended baseline:

- 1 `api-gateway` replica when `/api/v1/stream/live` is part of the supported operator workflow
- 2+ `portal` replicas
- 1+ `collector` replica, scaled horizontally for volume
- managed PostgreSQL with PITR enabled
- managed Redis with replication and failover
- external ingress or load balancer

If cross-replica live-stream fan-out is implemented and field-validated later, the gateway replica recommendation can be revisited. Until then, the supported default remains single-replica for complete live-stream delivery.

## Failure Domains

- `api-gateway` is stateless and can be replaced freely
- `portal` is stateless and can be replaced freely
- `collector` is mostly stateless; some in-flight telemetry can be retried
- PostgreSQL is the critical stateful dependency
- Redis affects performance and some runtime coordination, but not long-term evidence storage

Important exception:

- `/api/v1/stream/live` is not an HA-safe feature today; it uses an in-memory hub inside one `api-gateway` process
- if operators require complete live event delivery, run a single `api-gateway` replica for that path or treat live stream as unsupported in the HA topology
- Helm intentionally blocks `api.replicas > 1` unless `api.allowUnsupportedTopology=true` is set as an explicit override

## Kubernetes Controls

Use:

- pod disruption budgets from [../deploy/helm/templates/runtime-stack.yaml](../deploy/helm/templates/runtime-stack.yaml)
- network isolation from [../deploy/helm/templates/networkpolicies.yaml](../deploy/helm/templates/networkpolicies.yaml)
- readiness and liveness probes defined in the chart

Recommended additional posture:

- anti-affinity or topology spread for gateway and portal
- externalized secrets
- managed ingress or load balancer
- database monitoring and PITR validation

## Upgrade Approach

Recommended sequence:

1. back up PostgreSQL
2. apply migrations
3. roll `api-gateway`
4. roll `collector`
5. roll `portal`
6. run readiness, proxy, and GA gate checks

This keeps the control plane upgrade path predictable and auditable.

## Minimum Production Targets

- no single replica for portal
- gateway replica count must match the declared live-stream operating mode:
  - single replica when live stream is in scope
  - or a validated cross-replica fan-out design before claiming HA delivery for live stream
- database backups tested
- TLS termination defined
- OIDC or password-login policy agreed
- network policy enabled
- live stream expectations documented honestly: multi-replica gateway deployments do not provide complete `/api/v1/stream/live` delivery

## Operational Expectations

A high-availability posture should also include:

- documented rollback ownership
- clear backup and restore ownership
- staging validation before production rollout
- release artifacts attached to change review

## What Good Looks Like

A production-ready HA posture has:

- redundant gateway and portal instances
- resilient PostgreSQL and Redis services
- tested backup and restore
- validated rollout procedure
- release evidence captured before each production change

## Related Documents

Use this guide with:

- [BACKUP_RESTORE.md](BACKUP_RESTORE.md)
- [REFERENCE_DEPLOYMENT.md](REFERENCE_DEPLOYMENT.md)
- [PRODUCTION_CHECKLIST.md](PRODUCTION_CHECKLIST.md)
- [runbooks/NETPROXY_CA_ROTATION_RUNBOOK.md](runbooks/NETPROXY_CA_ROTATION_RUNBOOK.md)
