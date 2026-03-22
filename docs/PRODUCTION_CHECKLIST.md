# AgentFabric Production Checklist

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

- [ ] `AF_AUTH_DISABLED` is absent or `false`
- [ ] `AF_JWT_SECRET` is not a development sentinel
- [ ] `AF_ADMIN_PASSWORD` is not `admin`
- [ ] `AF_VAULT_KEY` is set to a 64-character hex value
- [ ] `DATABASE_URL` points to the production PostgreSQL instance
- [ ] `REDIS_URL` points to the production Redis instance
- [ ] `AF_CORS_ORIGINS` is restricted to the deployed portal origin or origins
- [ ] If `AF_TLS_ENABLED=true`, both `AF_TLS_CERT_FILE` and `AF_TLS_KEY_FILE` are present
- [ ] If `AF_SSO_REQUIRED=true`, OIDC settings are complete:
  - `AF_OIDC_ISSUER`
  - `AF_OIDC_CLIENT_ID`
  - `AF_OIDC_CLIENT_SECRET`
  - `AF_OIDC_REDIRECT_URI`
- [ ] If SSO cutover is complete, `AF_PASSWORD_LOGIN_DISABLED=true`
- [ ] Collector production config is complete:
  - `AF_GATEWAY_ENDPOINT`
  - `AF_GATEWAY_AUTH_TOKEN`
  - `AF_JWT_SECRET` when auth is required

## 3. Readiness and Startup

- [ ] Gateway returns `200` on `GET /healthz`
- [ ] Gateway returns `200` on `GET /readyz`
- [ ] Collector returns `200` on `GET /healthz`
- [ ] Collector returns `200` on `GET /readyz`
- [ ] Gateway startup completed migrations successfully
- [ ] Pricing rules loaded successfully at startup
- [ ] Policy rules loaded successfully at startup

## 4. Automated Validation

- [ ] Focused Go test suites pass in CI
- [ ] Portal tests pass in CI
- [ ] Portal production build passes in CI
- [ ] Swagger and OpenAPI checks pass
- [ ] Packaging validation passes for:
  - local Docker Compose
  - production Docker Compose overlay
  - Helm render or lint
- [ ] Stack probe passes:
  - `scripts/probe_stack_health.ps1`
  - `scripts/probe-stack-health.sh`
- [ ] Proxy-path probe passes:
  - `scripts/probe_proxy_path.ps1`
  - `scripts/probe-proxy-path.sh`
- [ ] `scripts/run_release_candidate_validation.ps1` or `scripts/run-release-candidate-validation.sh` passes against the candidate deployment
- [ ] Backup path passes:
  - `scripts/backup_postgres.ps1`
  - `scripts/backup-postgres.sh`

## 5. Runtime Smoke

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

## 6. Coverage and Rollout Reality

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

## 7. Security and Audit

- [ ] Pricing rule changes are auditable
- [ ] Policy rule changes are auditable
- [ ] Key create, rotate, and delete actions are auditable
- [ ] Admin actions are auditable
- [ ] DLP enforcement is visible in traces and audit when exercised
- [ ] Secret material is not exposed through admin list APIs
- [ ] Security headers are present on gateway responses

## 8. GA Decision Bar

Only declare GA when all of the following are true:

- [ ] CI is green
- [ ] Helm and Docker Compose packaging renders are green
- [ ] readiness, proxy-path, and release-candidate validation are green in a real staging environment
- [ ] governance scenarios are green
- [ ] docs match the deployed product and supported provider scope
- [ ] no open P0 or P1 release blockers remain
- [ ] latest successful backup is no older than 24 hours
- [ ] restore drill has been executed in the current release cycle

## 9. Final GA Gate

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
GA_CI_GREEN=true \
OPEN_P0_COUNT=0 \
OPEN_P1_COUNT=0 \
bash scripts/run-ga-gate.sh
```

## 10. Validation Commands

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
  -AdminPassword <admin-password>
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
bash scripts/run-release-candidate-validation.sh
```

## Related Documents

Use this checklist with:

- [REFERENCE_DEPLOYMENT.md](REFERENCE_DEPLOYMENT.md)
- [SETUP_AND_ONBOARDING.md](SETUP_AND_ONBOARDING.md)
- [RELEASE_BOUNDARIES.md](RELEASE_BOUNDARIES.md)
- [BACKUP_RESTORE.md](BACKUP_RESTORE.md)
