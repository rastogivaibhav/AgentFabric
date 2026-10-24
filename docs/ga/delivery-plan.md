# Enterprise GA Delivery Plan and Implementation Backlog

## 1. Delivery strategy

Enterprise GA is delivered through gated vertical slices, not parallel feature
accumulation. Each gate must produce a deployable increment and verified
evidence before the next layer expands product breadth.

The first production-shaped slice is deliberately narrow:

```text
one authenticated tenant and employee
  -> one versioned agent
  -> one durable task
  -> one model invocation
  -> one typed approval-gated action
  -> one SPACE or PUL effect
  -> one complete evidence chain and cost settlement
  -> restart/recovery and deny/bypass tests
```

Do not begin broad channel, mobile, marketplace, or multi-agent ecosystem work
until this slice survives security and restart acceptance.

## 2. Workstreams

| Workstream | Scope | Primary owner |
|---|---|---|
| WS1 Identity and tenancy | OIDC/MFA/SCIM, users/groups, RBAC/ABAC, RLS, support access | Identity/API team |
| WS2 Domain and contracts | Canonical IDs, agent/task/action/evidence schemas, generated clients | Architecture/platform API |
| WS3 Durable runtime | Coordinator, leases, state machines, outbox, retries, schedules | Agent runtime team |
| WS4 Governed action plane | Action Gateway, target resolution, AGS, approval, capabilities | Governance team |
| WS5 Execution and connections | SPACE broker/browser, PUL broker/live connectors, Model Gateway | Execution/integration teams |
| WS6 Evidence and FinOps | Evidence ledger, anchors, GovAgn projection, budgets, pricing | Evidence/observability team |
| WS7 Employee/admin product | Employee roster/tasks, CIO/CISO/Ops/Auditor surfaces | Product/UI team |
| WS8 Platform and air gap | Helm, HA, secrets, registries, offline bundle, upgrade/DR | Platform/SRE/release team |
| WS9 Quality and assurance | Contract tests, security, evals, performance, chaos, acceptance | QA/security test |

Each workstream has a product owner, engineering owner, security partner, SRE
partner, and acceptance owner. Component ownership cannot be implicit.

## 3. Gate 0 — Contain current critical risk

**Objective:** Make the current repository safe to continue developing and stop
documented bypass/tenant/privilege defects from becoming production defaults.

### Epics

#### G0-E1 Identity and tenant containment

- Fix self-service user update so role, tenant, groups, status, and privileged
  attributes are administrator-only.
- Constrain budget reads/writes to authenticated tenant; model platform admin.
- Remove caller-controlled collector tenant and bind ingest to workload identity.
- Add route inventory and cross-tenant tests for API, WebSocket, export, and UI.
- Make auth mandatory and weak/default secrets fatal in production mode.

Requirements: `PRD-IDN-*`, `SEC-IAM-*`, `SEC-TEN-*`.

Exit evidence:

- privilege escalation regression test;
- budget/telemetry cross-tenant regression tests;
- production configuration negative-startup tests;
- route-level authorization inventory signed by security.

#### G0-E2 Local runtime containment

- Bind local runtime to loopback and label it development-only.
- Add explicit startup refusal for production mode.
- Remove unsafe secret extraction from container environments.
- Add private/link-local/metadata/DNS-rebinding/redirect browser blocks.
- Disable unsupported production browser claims.
- Bind executable content digest into command permit.

Exit evidence:

- no non-loopback exposure in default developer launcher;
- SSRF and script substitution regression suite;
- documentation and UI accurately identify execution plane/proof limits.

#### G0-E3 Build and test baseline

- Repair dependency locks/checksums so API gateway, SDK, portal, af-core,
  collector, and provider suites run hermetically.
- Add enterprise-agent unit/API/security tests.
- Remove or quarantine active references to archived af-core audit code.
- Make CI run all supported component tests and publish immutable results.

Exit: clean checkout passes all mandatory unit/contract suites on supported
builders. No test lane depends on a developer's global packages.

## 4. Gate 1 — Canonical enterprise control plane

**Objective:** Join the employee agent product to authenticated AgentFabric
identity, PostgreSQL, portal, and canonical contracts.

### Epics

#### G1-E1 Approve architecture decisions

Approve ADRs for:

- AGS as sole execution PDP;
- canonical IDs/action/evidence protocols;
- PostgreSQL durable coordinator;
- forced RLS and database roles;
- SPACE production computer/browser provider;
- PUL exclusive connection broker;
- evidence separate from telemetry;
- signed catalogues and offline release BOM;
- no employee YOLO and constrained break-glass.

#### G1-E2 Consolidate data model

- Inventory and reconcile duplicate `agent_runs/runs`, `span_metadata/spans`,
  feedback, user, budget, audit, and price schemas.
- Adopt one UUID tenant model and non-owner app/migration/audit roles.
- Add organisations, memberships, agents/versions/instances, catalogues,
  entitlements, tasks/runs/steps, actions/approvals/receipts/evidence,
  budget reservations/settlements, outbox/inbox/leases/dead letters.
- Implement additive migration, validation, dual-read or backfill plan, cutover,
  and retirement of legacy tables.

#### G1-E3 Authenticated agent APIs and UI

- Implement `/v1/agents`, templates/catalogues, and lifecycle desired state.
- Use generated authenticated clients in the React portal.
- Add employee roster/create/detail experiences.
- Split employee and privileged admin routes/surfaces.
- Retain inline FastAPI UI only as a development fixture pending deletion.

#### G1-E4 Provisioning reconcilers

- Persist desired agent state before external calls.
- Reconcile AGS identity, PUL app identity, evidence identity, and workspace.
- Implement deterministic idempotency, compensation, retry, quarantine, and
  orphan discovery.

### Gate 1 exit

- Employee SSO/tenant/role controls protect all new agent APIs.
- Forced RLS negative tests pass.
- Agent provisioning recovers from failure at every step without duplicates.
- One auditor can follow canonical IDs across all provisioned systems.
- Current local user-visible behaviour is available through the supported portal
  without unauthenticated production endpoints.

## 5. Gate 2 — Durable agent runtime

**Objective:** Replace synchronous request-bound turns with persistent,
recoverable task execution.

### Epics

#### G2-E1 Coordinator and task state machine

- Idempotent task/message/steer/pause/resume/cancel commands.
- PostgreSQL leases using database time and `SKIP LOCKED`.
- Immutable transition history, optimistic versions, checkpoints, deadlines.
- Transactional outbox and idempotent inbox consumers.
- Retry classification, backoff, attempt budget, dead letters, reconciliation.

#### G2-E2 Planner workers and Model Gateway

- Extract planning from the web process into unprivileged workers.
- Define bounded agent loop, structured model output, tool/action schemas.
- Implement model profiles, internal Ollama-compatible provider first, usage
  reservation/settlement, streaming, fallback restrictions, redaction.
- Add evidence-required completion checks and explicit limitations.

#### G2-E3 Durable approval/input/cancellation

- Pause and resume across process/node restart.
- Notification outbox, expiry, reminder, deny/withdraw/handoff.
- Fresh identity and separation-of-duty enforcement.
- Cancellation checkpoints and active execution termination intent.

#### G2-E4 Schedules and delivery ledger

- Timezone/DST, recurrence, concurrency, missed-run, retry, expiry.
- Scheduled workload identity and narrower entitlements.
- Durable outbound delivery states including unknown.

### Gate 2 exit

- Restart and node loss do not lose acknowledged input or tasks.
- Approval waits survive deployment.
- Forced ambiguous effects are reconciled without duplicates.
- Cancellation, deadlines, retries, and dead letters are employee/operator visible.
- Model use is exclusively through the gateway with pre-use reservation.

## 6. Gate 3 — Unavoidable governed action plane

**Objective:** Make every supported effect and sensitive read pass through the
same trusted action transaction.

### Epics

#### G3-E1 Canonical Action Gateway

- Versioned capability registry and schemas.
- Trusted target resolution and semantic effect classification.
- Content/action digest, data/purpose/destination/resource enrichment.
- Persist-before-call, idempotency, terminal receipt validation, unknown state.
- Result validation, bounded output, redaction, artifact handling.

#### G3-E2 AGS and approval integration

- Signed action-bound decision contract.
- Deny precedence, fail-closed transport/schema/expiry/revocation.
- Policy baseline, version, promotion, rollback, shadow testing.
- Digest-bound approval with re-resolution and reauthorization.

#### G3-E3 PUL production broker

- Separate provisioning and runtime identities/endpoints.
- Vaulted connection lifecycle and least-privilege operation grants.
- At least one read connector and one approval-gated write connector.
- Signed receipts, idempotency/reconciliation, revoke, error/secret controls.

#### G3-E4 SPACE production broker and browser

- Narrow domain API; no raw Docker/KVM management.
- Action-bound capability verification and template/software/network constraints.
- Production browser in SPACE or equivalently certified isolated broker.
- Network-boundary egress, DNS/redirect/metadata protection, file scanning.
- Workspace retention, snapshot, teardown, orphan reconciliation.

#### G3-E5 Lossless evidence ledger

- Multi-writer-safe append-only chain/tree and signatures.
- Proposal-before-effect and outcome receipt.
- Artifact manifests and external WORM/timestamp anchor.
- Verification/export, completeness monitor, restore continuity.
- GovAgn/ClickHouse projections through outbox, not as evidence authority.

### Gate 3 exit

- Automated bypass matrix proves no supported direct effect route.
- Planner compromise yields no administrative secrets or execution management.
- Real AGS allow/deny/approval and real PUL/SPACE effects produce a complete,
  externally anchored evidence chain.
- Unknown execution and evidence-store failures fail safely.

## 7. Gate 4 — Enterprise controls, HA, and product completion

**Objective:** Complete enterprise administration, isolation, scale, and
operational recovery.

### Epics

#### G4-E1 Enterprise catalogues and extensibility

- Signed models, skills, tools, MCP, software, computer, channel catalogues.
- Admission/promotion/revocation/support lifecycle.
- Out-of-process extension sandbox and provider-driver contract testkit.
- Progressive skill disclosure without implicit authority.

#### G4-E2 Controlled memory and knowledge

- Scoped/provenance/classification/retention model.
- Reviewed automatic writes and user correction.
- ACL-preserving retrieval and deletion/index propagation.
- Legal hold, export, evidence, and contamination evaluation.

#### G4-E3 Administration experiences

- CIO: catalogue, capacity, quotas, budget, releases, service health.
- CISO: policy, approvals, classifications, egress, quarantine, revocation,
  emergency stop, evidence completeness.
- Operations: queues, workers, connectors, SPACE/model pools, backups, DR.
- Auditor: read-only search, verify, export, hold/retention, access evidence.

#### G4-E4 HA and scale

- Multi-replica stateless APIs/workers/brokers.
- PostgreSQL/event/object/evidence/model/SPACE HA per support profile.
- Durable live-event fan-out and client resume cursor.
- Hierarchical admission, fairness, rate, quotas, noisy-neighbour controls.
- Reference load, stress, soak, burst, failover, chaos.

#### G4-E5 Incident and emergency controls

- Kill/quarantine/revoke by tenant, agent, connection, tool, model, template,
  destination, policy, workload.
- Dual-control break-glass with expiry and evidence.
- Incident timelines, export, SIEM, and response runbooks/tabletops.

### Gate 4 exit

- SLO/reference load and failure-domain tests pass with headroom.
- Backup/restore and DR meet RPO/RTO and evidence continuity.
- Product/admin critical journeys pass accessibility/usability acceptance.
- All alerts, dashboards, owners, and runbooks are operationally exercised.

## 8. Gate 5 — Signed enterprise and air-gap release

**Objective:** Deliver one supportable product release with no hidden internet
or source-checkout dependency.

### Epics

#### G5-E1 Consolidated deployment

- Complete Helm workloads, service accounts, policies, migrations, monitors.
- Evaluation Compose clearly separated from production.
- Support matrix and preflight for Kubernetes/Linux/KVM/storage/PKI/IdP.
- GitOps configuration and drift reporting.

#### G5-E2 Hermetic release and offline bundle

- Immutable multi-architecture images/installers and component BOM.
- SBOM, provenance, signatures, licences, vulnerability snapshot.
- SPACE/browser/templates, models, skills/tools/software catalogues.
- Offline verify/import/install/status/diagnostic utilities.

#### G5-E3 Upgrade, rollback, backup, and DR

- N-1 compatibility and canary rollout.
- Migration/rollback boundaries and mixed-version prevention.
- Automated backup, isolated restore, and disaster-recovery exercises.
- Certificate/key/policy/catalogue/model rotation.

#### G5-E4 Independent GA acceptance

- Exact release candidate deployed from offline bundle.
- Real end-to-end employee, approval, connector, computer/browser, evidence,
  FinOps, incident, and recovery journeys.
- Security, eval, load, chaos, penetration, accessibility, and documentation.
- Signed evidence pack and residual risk acceptance.

### Gate 5 exit

All criteria in [Verification and GA Acceptance](verification-acceptance.md)
are satisfied by the exact signed release artifacts. Product, Architecture,
CISO, SRE, and Independent Acceptance approve release.

## 9. Post-GA expansion

After GA controls and operations are stable:

- additional PUL connectors and enterprise channels;
- desktop and managed mobile clients;
- richer team/delegation patterns;
- broader model and hardware providers;
- industry-specific policy/control packs;
- multi-site read replicas and future active/active investigation;
- an approved internal extension marketplace.

Post-GA features use the same action, identity, evidence, catalogue, release,
and verification contracts. No alternate agent core is introduced per channel.

## 10. Current repository mapping

| Current asset | Target use | Required change |
|---|---|---|
| `enterprise-agent/app.py` | Behavioural prototype/test fixture | Decompose; retire as production server |
| `integrations/space_provider` | SPACE/AGS/PUL broker contract seed | Move behind services; PostgreSQL/evidence/capabilities |
| `api-gateway` | Identity/API edge seed | Fix P0 authz/tenant issues; add agent/task BFF/contracts |
| `collector` | Telemetry ingest | Workload-bound tenant; durable loss visibility |
| `af-core` | Telemetry/risk analytics | Rename semantics; remove execution-policy/audit claims |
| `portal` | Employee/admin web foundation | Generated client; role-separated experiences |
| `agent-sdk` | Framework instrumentation | Hermetic dependencies; new canonical IDs/evidence refs |
| `deploy/migrations` | Schema baseline | Consolidate duplicate schema and force RLS |
| root Compose | Integration/evaluation environment | No production claim; align with BOM |
| `deploy/helm` | Production packaging seed | Add complete workloads/policies/migrations |
| GovAgn external stack | Evidence/ops/FinOps projection | Versioned adapter/BOM; not sole evidence authority |
| AGS/PUL/SPACE external products | Policy/capability/execution planes | Versioned contracts, independent audit, supported BOM |

## 11. Dependency graph

```text
G0 containment
  -> canonical IDs/contracts and identity/RLS (G1)
      -> durable coordinator/model gateway (G2)
          -> unavoidable action/AGS/PUL/SPACE/evidence (G3)
              -> product breadth, HA, admin controls (G4)
                  -> signed offline release and independent acceptance (G5)
```

Parallelism is safe within a gate only when upstream contracts are approved.
UI mocks, provider adapters, performance harnesses, and packaging pipelines can
start early, but they cannot invent alternate domain or authority models.

## 12. Definition of Ready

A GA backlog item is ready when it has:

- linked requirement IDs and journey;
- explicit user/operator outcome and non-goals;
- owner and reviewers;
- API/event/data/security contract or statement of no contract change;
- threat-model and tenant/data impact;
- failure, retry, idempotency, cancellation, and rollback behaviour;
- observability/evidence and support implications;
- automated/manual acceptance cases and test environment;
- dependencies and migration/compatibility plan.

## 13. Definition of Done

An item is done only when:

- code/config/schema/docs are reviewed by required owners;
- unit, contract, integration, security, failure, and regression tests pass;
- tenant and privilege negative cases are included;
- logs/metrics/traces/evidence are implemented, bounded, and redacted;
- runbook, alerts, dashboards, capacity/limits, and rollback are updated;
- migration and N/N-1 compatibility are validated when applicable;
- artifacts are signed/SBOMed/provenanced through the release pipeline;
- requirement traceability and release evidence are updated;
- no mock/dry-run behaviour is represented as production capability.

## 14. Architecture governance

Changes require ADR review when they affect:

- identity/tenant/role model;
- state ownership or consistency;
- action/evidence schemas;
- AGS/PUL/SPACE/GovAgn responsibility;
- privileged process or trust boundary;
- new effect, tool, extension, data class, destination, or channel;
- production dependency or deployment profile;
- encryption/signing/key custody;
- SLO/RPO/RTO, retention, or compatibility.

The Architecture Review Board includes enterprise, solution, security,
infrastructure, data, and product representatives. CISO approval is mandatory
for changes to non-negotiable security invariants.

## 15. Programme reporting

Report progress by proven gate outcomes, not story count. The dashboard shows:

- mandatory requirements implemented/tested/blocked/waived;
- open P0/P1 defects and residual risks;
- end-to-end journeys passing with real vs fixture dependencies;
- tenant/action/evidence bypass coverage;
- restart/recovery/unknown-action results;
- SLO/capacity/DR/air-gap acceptance posture;
- release artifact/BOM/provenance readiness;
- expiring exceptions and accountable owners.

No percentage-complete metric may hide a failed GA invariant.

