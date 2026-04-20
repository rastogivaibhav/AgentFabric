# Govagn Production Checklist

**Audience:** release managers, platform owners, security reviewers  
**Applies to:** the current central deployment model of `api-gateway + collector + portal + PostgreSQL + Redis`

Use this checklist before every production deployment and before declaring the platform GA-ready for a shared organization or department rollout.

## 1. Release Scope

- [ ] Release scope is explicitly limited to the current supported provider path:
  - `openai`
  - `anthropic`
  - `google`
- [ ] Release notes do not advertise unsupported gateway capabilities or product areas outside the current release boundary.
- [ ] Deployment topology matches the supported reference shape:
  - shared `api-gateway`
  - shared `collector`
  - shared `portal`
  - shared PostgreSQL
  - shared Redis

## 2. Production Configuration

- [ ] `GV_AUTH_DISABLED` is absent or `false`
- [ ] `GV_JWT_SECRET` is not a development sentinel
- [ ] `GV_ADMIN_PASSWORD` is not `admin`
- [ ] `GV_VAULT_KEY` is set to a 64-character hex value
- [ ] `DATABASE_URL` points to the production PostgreSQL instance
- [ ] `REDIS_URL` points to the production Redis instance
- [ ] `GV_CORS_ORIGINS` is restricted to the deployed portal origin or origins
- [ ] `GV_NETPROXY_CA_CERT_FILE` and `GV_NETPROXY_CA_KEY_FILE` point at the persisted production CA files
- [ ] If `GV_TLS_ENABLED=true`, both `GV_TLS_CERT_FILE` and `GV_TLS_KEY_FILE` are present
- [ ] If `GV_SSO_REQUIRED=true`, OIDC settings are complete:
  - `GV_OIDC_ISSUER`
  - `GV_OIDC_CLIENT_ID`
  - `GV_OIDC_CLIENT_SECRET`
  - `GV_OIDC_REDIRECT_URI`
  - optional `GV_OIDC_LOGOUT_URL` reviewed for provider logout behavior
- [ ] If using Helm with `GV_SSO_REQUIRED=true`, the Secret named by `secrets.name` contains the key from `auth.oidc.clientSecretKey` (default `oidc-client-secret`)
- [ ] If SSO cutover is complete, `GV_PASSWORD_LOGIN_DISABLED=true`
- [ ] If operators depend on `/api/v1/stream/live`, the deployment uses a single `api-gateway` replica; multi-replica gateway deployments are not a supported complete-delivery topology for live stream today
- [ ] If Envoy egress interception is enabled:
  - Envoy listener health is green
  - workload trust stores include the Govagn CA root
  - DNS steering for managed domains points to Envoy egress
  - Envoy -> collector OTLP gRPC path is reachable
- [ ] Collector production config is complete:
  - `GV_ENV=production` or `GV_STRICT_CONFIG=true`
  - `GV_GATEWAY_ENDPOINT`
  - `GV_GATEWAY_AUTH_TOKEN` is present on both collector and gateway with the same value
  - `GV_AUTH_REQUIRE_AUTH=true`
  - `GV_JWT_SECRET`
  - `GV_GATEWAY_ENDPOINT` is not left at the built-in `http://localhost:8080` default

## 3. Readiness and Startup

- [ ] Gateway returns `200` on `GET /healthz`
- [ ] Gateway returns `200` on `GET /readyz`
- [ ] Collector returns `200` on `GET /healthz`
- [ ] Collector returns `200` on `GET /readyz`
- [ ] Gateway startup completed migrations successfully
- [ ] Pricing rules loaded successfully at startup
- [ ] Policy rules loaded successfully at startup

## 4. Merge-Gate CI Evidence

- [ ] Collector CI runs `go test ./...`
- [ ] API gateway CI runs `go test ./...`
- [ ] Portal tests pass in CI
- [ ] Portal production build passes in CI
- [ ] Portal Playwright smoke tests pass in CI
- [ ] Swagger and OpenAPI smoke checks pass in CI
- [ ] Packaging validation passes in CI for:
  - local Docker Compose render
  - production Docker Compose overlay render
  - Helm lint or template smoke
- [ ] Secret scan passes in CI
- [ ] Release docs and release-boundary docs stay aligned with the current runtime shape
- [ ] If the release changes `agent-sdk`, framework patching, or provider-compatibility claims, the latest `sdk-integration.yml` workflow run is green

## 5. Candidate Deployment Validation

- [ ] Stack probe passes:
  - `scripts/probe_stack_health.ps1`
  - `scripts/probe-stack-health.sh`
- [ ] Proxy-path probe passes:
  - `scripts/probe_proxy_path.ps1`
  - `scripts/probe-proxy-path.sh`
- [ ] `scripts/run_release_candidate_validation.ps1` or `scripts/run-release-candidate-validation.sh` passes against the candidate deployment with admin credentials
- [ ] `scripts/run_production_deployment_validation.ps1` or `scripts/run-production-deployment-validation.sh` passes against the candidate deployment and writes a release report
- [ ] If governance is part of the go-live bar, release-candidate validation is run with governance scenarios enabled and a real proxy virtual key
- [ ] Backup path passes for the release window:
  - `scripts/backup_postgres.ps1`
  - `scripts/backup-postgres.sh`
- [ ] NetProxy CA backup and restore drill passes for the release window:
  - `scripts/exercise_netproxy_ca_backup_restore.ps1`
  - `scripts/exercise-netproxy-ca-backup-restore.sh`

## 6. Runtime Smoke

- [ ] Admin login works
- [ ] `/auth/me` works with the browser or session flow
- [ ] Pricing preview works
- [ ] Policy preview works
- [ ] API key registration works for supported providers
- [ ] One proxy request creates a trace with:
  - model attribution
  - token usage
  - non-zero cost
- [ ] One proxy-path proof run verifies:
  - pricing preview
  - policy preview
  - proxied request success
  - trace visibility
  - trace cost visibility
  - control-audit visibility
- [ ] One budget-limited scenario is exercised and recorded
- [ ] One policy event is visible in the portal
- [ ] One control-plane audit event is visible in the portal

## 7. Coverage and Rollout Reality

- [ ] Rollout owners understand the product coverage boundary:
  - SDK-instrumented apps are covered
  - OTLP-producing services are covered
  - proxied LLM traffic is covered
- [ ] Stakeholders understand what is not automatic:
  - arbitrary hosts without onboarding are not covered
  - generic host or process monitoring is not the product goal
- [ ] Tenant onboarding defaults are prepared:
  - pricing defaults
  - policy defaults
  - authentication path
  - key-management approach

## 8. Security and Audit

- [ ] Pricing rule changes are auditable
- [ ] Policy rule changes are auditable
- [ ] Key create, rotate, and delete actions are auditable
- [ ] Admin actions are auditable
- [ ] DLP enforcement is visible in traces and audit when exercised
- [ ] Secret material is not exposed through admin list APIs
- [ ] Security headers are present on gateway responses

## 9. GA Decision Bar

Only declare GA when all of the following are true:

- [ ] CI is green
- [ ] Helm and Docker Compose packaging renders are green
- [ ] readiness, proxy-path, and release-candidate validation are green in a real staging environment
- [ ] governance scenarios are green
- [ ] if the release changes SDK patching or provider compatibility, the latest `sdk-integration.yml` run is green
- [ ] docs match the deployed product and supported provider scope
- [ ] no open P0 or P1 release blockers remain
- [ ] latest successful backup is no older than 24 hours
- [ ] restore drill has been executed in the current release cycle
- [ ] NetProxy CA backup and restore drill report shows `Validation result: PASS`
- [ ] production deployment validation report shows `Validation result: PASS`

## 10. Final GA Gate

Use the GA gate scripts for the final objective release decision. `ci` mode is for CI evidence. `ga` mode is the actual release decision and must be run with explicit blocker counts plus staging credentials.

- [ ] `scripts/run_ga_gate.ps1` passes in `ga` mode
- [ ] or `scripts/run-ga-gate.sh` passes in `ga` mode
- [ ] GA gate summary is attached to the release review and shows `GO`

### Windows PowerShell

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run_ga_gate.ps1 `
  -Mode ga `
  -BaseUrl http://<gateway-host>:8080 `
  -CollectorUrl http://<collector-host>:4318 `
  -AdminUser <admin-user> `
  -AdminPassword <admin-password> `
  -ProxyVirtualKey <af-vk-key> `
  -ProductionValidationReportPath .\production-deployment-validation.md `
  -NetProxyCaDrillReportPath .\netproxy-ca-drill.md `
  -CiGreen `
  -OpenP0Count 0 `
  -OpenP1Count 0
```

### macOS / Linux

```bash
GA_GATE_MODE=ga \
BASE_URL=http://<gateway-host>:8080 \
COLLECTOR_URL=http://<collector-host>:4318 \
ADMIN_USER=<admin-user> \
ADMIN_PASSWORD=<admin-password> \
PROXY_VIRTUAL_KEY=<af-vk-key> \
PRODUCTION_VALIDATION_REPORT_PATH=./production-deployment-validation.md \
NETPROXY_CA_DRILL_REPORT_PATH=./netproxy-ca-drill.md \
GA_CI_GREEN=true \
OPEN_P0_COUNT=0 \
OPEN_P1_COUNT=0 \
bash scripts/run-ga-gate.sh
```

## 11. Validation Commands

### Windows PowerShell

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\probe_stack_health.ps1 `
  -BaseUrl http://<gateway-host>:8080 `
  -CollectorUrl http://<collector-host>:4318

powershell -ExecutionPolicy Bypass -File .\scripts\probe_proxy_path.ps1 `
  -BaseUrl http://<gateway-host>:8080 `
  -AdminUser <admin-user> `
  -AdminPassword <admin-password> `
  -ProxyVirtualKey <af-vk-key>

powershell -ExecutionPolicy Bypass -File .\scripts\run_release_candidate_validation.ps1 `
  -BaseUrl http://<gateway-host>:8080 `
  -AdminUser <admin-user> `
  -AdminPassword <admin-password> `
  -RunGovernanceScenarios `
  -ProxyVirtualKey <af-vk-key>

powershell -ExecutionPolicy Bypass -File .\scripts\run_production_deployment_validation.ps1 `
  -BaseUrl http://<gateway-host>:8080 `
  -CollectorUrl http://<collector-host>:4318 `
  -AdminUser <admin-user> `
  -AdminPassword <admin-password> `
  -ProxyVirtualKey <af-vk-key> `
  -DatabaseUrl "postgres://user:pass@host:5432/govagn?sslmode=require" `
  -NetProxyCaCertFile C:\secure\govagn\netproxy-ca.crt `
  -NetProxyCaKeyFile C:\secure\govagn\netproxy-ca.key `
  -LiveStreamSingleReplica
```

### macOS / Linux

```bash
BASE_URL=http://<gateway-host>:8080 \
COLLECTOR_URL=http://<collector-host>:4318 \
bash scripts/probe-stack-health.sh

BASE_URL=http://<gateway-host>:8080 \
ADMIN_USER=<admin-user> \
ADMIN_PASSWORD=<admin-password> \
bash scripts/probe-proxy-path.sh

BASE_URL=http://<gateway-host>:8080 \
ADMIN_USER=<admin-user> \
ADMIN_PASSWORD=<admin-password> \
RUN_GOVERNANCE_SCENARIOS=true \
PROXY_VIRTUAL_KEY=<af-vk-key> \
bash scripts/run-release-candidate-validation.sh

BASE_URL=http://<gateway-host>:8080 \
COLLECTOR_URL=http://<collector-host>:4318 \
ADMIN_USER=<admin-user> \
ADMIN_PASSWORD=<admin-password> \
PROXY_VIRTUAL_KEY=<af-vk-key> \
DATABASE_URL="postgres://user:pass@host:5432/govagn?sslmode=require" \
NETPROXY_CA_CERT_FILE=/secure/govagn/netproxy-ca.crt \
NETPROXY_CA_KEY_FILE=/secure/govagn/netproxy-ca.key \
LIVE_STREAM_SINGLE_REPLICA=true \
bash scripts/run-production-deployment-validation.sh
```

## Related Documents

Use this checklist with:

- [REFERENCE_DEPLOYMENT.md](REFERENCE_DEPLOYMENT.md)
- [SETUP_AND_ONBOARDING.md](SETUP_AND_ONBOARDING.md)
- [RELEASE_BOUNDARIES.md](RELEASE_BOUNDARIES.md)
- [BACKUP_RESTORE.md](BACKUP_RESTORE.md)
- [runbooks/NETPROXY_CA_ROTATION_RUNBOOK.md](runbooks/NETPROXY_CA_ROTATION_RUNBOOK.md)
