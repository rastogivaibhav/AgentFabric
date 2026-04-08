# Envoy Egress Deployment

## Purpose

This document defines the Govagn Envoy egress pattern for network-level interception of outbound LLM traffic.

Target domains:

- `api.openai.com`
- `api.anthropic.com`
- `generativelanguage.googleapis.com`

The Envoy egress gateway:

- terminates downstream TLS with certificates signed by a local/private CA
- inspects HTTP request/response metadata for policy and telemetry use
- re-originates upstream TLS to provider endpoints
- exports Envoy traces to the Govagn collector over OTLP gRPC (`collector:4317`)

## Artifacts

- Envoy config: [../deploy/envoy/envoy.yaml](../deploy/envoy/envoy.yaml)
- Envoy image: [../deploy/envoy/Dockerfile](../deploy/envoy/Dockerfile)
- Compose overlay: [../deploy/docker/docker-compose.envoy.yml](../deploy/docker/docker-compose.envoy.yml)
- K8s starter manifest: [../deploy/envoy/envoy-egress-gateway.yaml](../deploy/envoy/envoy-egress-gateway.yaml)
- Cert generation: [../scripts/generate-envoy-egress-certs.sh](../scripts/generate-envoy-egress-certs.sh)

## Local Bring-Up

1. Generate base CA certs if needed:
   - `bash scripts/generate-dev-certs.sh`
2. Generate provider-domain leaf certs:
   - `bash scripts/generate-envoy-egress-certs.sh`
3. Start core stack:
   - `docker compose -f docker-compose.yml up -d --build`
4. Start Envoy egress profile:
   - `docker compose -f docker-compose.yml -f deploy/docker/docker-compose.envoy.yml --profile envoy up -d --build`
5. Verify Envoy admin:
   - `curl http://localhost:9901/ready`

The provided `envoy.yaml` uses `collector:4317` for OTLP export (works in Docker Compose). For Kubernetes or VPC deployments, update the `otlp_collector` cluster endpoint to your collector service DNS name.

## Traffic Steering Requirements

To perform TLS interception, workloads must:

- trust `deploy/certs/ca.crt`
- resolve the target provider domains to the Envoy egress IP or service address
- preserve original SNI/Host values (`api.openai.com`, `api.anthropic.com`, `generativelanguage.googleapis.com`)

Without CA trust and DNS steering, Envoy cannot terminate and inspect TLS payloads.

## AWS VPC Pattern

Recommended pattern:

1. Deploy Envoy as an egress tier behind an internal NLB.
2. Publish private hosted zone overrides for the three target domains to the NLB.
3. Attach application subnets to route tables that direct those domain lookups to the private zone.
4. Distribute the private CA root (`ca.crt`) to all workloads that call LLM providers.
5. Allow outbound 443 from Envoy to provider endpoints, and allow Envoy -> collector (`4317/tcp`) inside the VPC.

Operational notes:

- Scope DNS overrides only to approved workloads/accounts.
- Keep CA private key in a KMS/HSM-backed secret manager, not on application nodes.
- Rotate leaf certificates and CA on a documented schedule.

## Kubernetes Pattern

Recommended pattern:

1. Deploy Envoy egress as a dedicated Deployment/Service in a shared platform namespace.
2. Mount Envoy config and MITM certs from ConfigMap + Secret.
3. Add CoreDNS rewrite/forward rules (or mesh egress policies) so target domains resolve to the Envoy Service.
4. Mount trusted CA bundle into workloads (or update node trust store) so TLS interception validates.
5. Restrict outbound NetworkPolicies so application pods cannot bypass Envoy for the managed domains.

Starter command example:

- `kubectl apply -f deploy/envoy/envoy-egress-gateway.yaml`

Create/update the required ConfigMap from `deploy/envoy/envoy.yaml`:

- `kubectl -n govagn-system create configmap govagn-envoy-egress-config --from-file=envoy.yaml=deploy/envoy/envoy.yaml --dry-run=client -o yaml | kubectl apply -f -`

## Validation

After deployment:

- Envoy `/ready` returns `200`
- Envoy access logs show requests for the three provider authorities
- collector receives Envoy spans on OTLP gRPC
- proxy and policy workflows still pass release validation scripts
