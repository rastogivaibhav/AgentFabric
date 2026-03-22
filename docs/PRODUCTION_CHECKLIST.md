# AgentFabric Production Checklist

**Audience**: release managers, platform owners, security reviewers  
**Applies to**: the current central deployment model of `api-gateway + collector + portal + PostgreSQL + Redis`

Use this checklist before every production deployment and before declaring the platform GA-ready for a shared org or department rollout.

## 1. Release Scope

- [ ] Release scope is explicitly limited to the current supported provider path:
  - `openai`
  - `anthropic`
- [ ] Release notes do not advertise unsupported gateway capabilities or research/eval features that are not in the product boundary.
- [ ] Deployment topology matches the supported reference shape:
  - shared `api-gateway`
  - shared `collector`
  - shared `portal`
  - shared PostgreSQL
  - shared Redis

## 2. Production Configuration

- [ ] `AF_AUTH_DISABLED` is absent or `false`
- [ ] `AF_JWT_SECRET` is not the development sentinel
- [ ] `AF_ADMIN_PASSWORD` is not `admin`
- [ ] `AF_VAULT_KEY` is set to a 64-character hex value
- [ ] `DATABASE_URL` points to the production PostgreSQL instance
- [ ] `REDIS_URL` points to the production Redis instance
- [ ] `AF_CORS_ORIGINS` is restricted to the deployed portal origin(s)
- [ ] If `AF_TLS_ENABLED=true`, both `AF_TLS_CERT_FILE` and `AF_TLS_KEY_FILE` are present
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
- [ ] Swagger/OpenAPI checks pass
- [ ] Packaging validation passes for:
  - local Docker Compose
  - production Docker Compose overlay
  - Helm render/lint
- [ ] `scripts/run_release_candidate_validation.ps1` or `scripts/run-release-candidate-validation.sh` passes against the candidate deployment

## 5. Runtime Smoke

- [ ] Admin login works
- [ ] `/auth/me` works with the browser/session flow
- [ ] Pricing preview works
- [ ] Policy preview works
- [ ] API key registration works for supported providers
- [ ] One proxy request creates a trace with:
  - model attribution
  - token usage
  - non-zero cost
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
  - generic host/process monitoring is not the product goal
- [ ] Tenant onboarding defaults are prepared:
  - pricing defaults
  - policy defaults
  - authentication path
  - key-management approach

## 7. Security and Audit

- [ ] Pricing rule changes are auditable
- [ ] Policy rule changes are auditable
- [ ] Key create/rotate/delete actions are auditable
- [ ] Admin actions are auditable
- [ ] DLP enforcement is visible in traces and audit when exercised
- [ ] Secret material is not exposed through admin list APIs

## 8. GA Decision Bar

Only declare GA when all of the following are true:

- [ ] release validation is repeatable
- [ ] readiness and smoke checks are green in a real staging environment
- [ ] governance behavior is demonstrated, not inferred
- [ ] docs match the deployed product
- [ ] no open P0 or P1 release blockers remain

## 9. Validation Commands

Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run_release_candidate_validation.ps1 `
  -BaseUrl http://<gateway-host>:8080 `
  -AdminUser <admin-user> `
  -AdminPassword <admin-password>
```

macOS / Linux:

```bash
BASE_URL=http://<gateway-host>:8080 \
ADMIN_USER=<admin-user> \
ADMIN_PASSWORD=<admin-password> \
bash scripts/run-release-candidate-validation.sh
```
