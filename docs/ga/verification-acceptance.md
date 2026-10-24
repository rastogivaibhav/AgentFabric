# Enterprise GA Verification and Acceptance Specification

## 1. Purpose

This document defines the evidence required to declare Enterprise GA. Testing
must prove the platform's business outcome and control invariants under normal,
malicious, overloaded, degraded, restarted, upgraded, restored, and disconnected
conditions. Passing unit tests alone is insufficient.

## 2. Verification principles

- Test supported production artifacts, not only source-tree development modes.
- Treat identity, policy, approval, execution, and evidence as one end-to-end
  security transaction.
- Use real AGS, PUL, SPACE, GovAgn, model runtime, IdP, data stores, and at least
  one live controlled connector for GA acceptance.
- Preserve mocks/fixtures for deterministic contract tests but label them.
- Test denials and failures as first-class product behaviour.
- Verify absence of bypass, duplicate effect, secret leakage, cross-tenant
  access, and false success.
- Produce signed, reproducible evidence from immutable release artifacts.

## 3. Test environments

| Environment | Purpose | Required fidelity |
|---|---|---|
| Developer | Fast unit and component feedback | Mocks permitted; no GA evidence |
| Integration | Real PostgreSQL/event/object stores plus service contracts | Real services; external providers may use controlled fixtures |
| Security | Adversarial testing and instrumented attack simulation | Production-equivalent network/identity/isolation |
| Performance | Reference and stress capacity | Production topology and representative data |
| Release candidate | Full acceptance of exact signed artifacts | Exact BOM, charts, migrations, clients, policies |
| Air-gap acceptance | Disconnected install and operation | Physical/admin network block; internal dependencies only |
| DR recovery | Backup/restore and site-loss exercises | Separate recovery environment/failure domain |

- **TST-ENV-001:** Production data and credentials MUST NOT be copied into test.
- **TST-ENV-002:** Release-candidate and air-gap environments MUST deploy only
  signed artifacts from the candidate bundle.
- **TST-ENV-003:** Environment drift, versions, configuration digests, hardware,
  kernel, policies, and test-data manifests MUST be captured in results.

## 4. Test layers

### 4.1 Unit and property tests

Required for:

- domain validation and state transitions;
- tenant/identity context parsing;
- canonical serialization and digest calculation;
- policy/decision/receipt validators;
- idempotency and retry classification;
- redaction, classification, price calculation, quota reservation/settlement;
- evidence hashing, signature, ordering, export, and verification;
- URL/DNS/redirect/network target resolution;
- schedule calculations, timezone/DST, missed-run and overlap policy;
- retention, legal hold, and deletion selection;
- every adapter's success, deny, timeout, malformed, redirect, oversized,
  partial, and unknown response paths.

- **TST-UNIT-001:** Security and state-machine modules MUST use table-driven or
  property-based tests for invalid transitions and adversarial inputs.
- **TST-UNIT-002:** Coverage thresholds are set per critical package; coverage
  percentage MUST NOT replace required behavioural cases.
- **TST-UNIT-003:** No flaky test is silently retried in release gates; retries
  are reported and owned.

### 4.2 Contract tests

Contract suites must cover:

- API Gateway/BFF OpenAPI schemas and auth/error semantics;
- task commands, events, durable cursors, and idempotency;
- Model Gateway providers;
- AGS decisions, signatures, constraints, expiry, revocation, and fail-closed;
- PUL provisioning/runtime separation, approvals, receipts, reconciliation;
- SPACE lifecycle, commands, browser, files, network, snapshots, cleanup;
- GovAgn/evidence ingestion, query, verification, and export;
- channel delivery and unknown-send semantics;
- compatibility across N/N-1 declared versions.

- **TST-CON-001:** Provider contract tests MUST use scripted fake services/CLIs
  and include bounded output, stderr, non-zero exit, signal, timeout, and crash.
- **TST-CON-002:** Consumer-driven contracts MUST be versioned and run against
  every supported independently deployed component version in the BOM.
- **TST-CON-003:** A contract test pass is not recorded as live service proof.

### 4.3 Integration and end-to-end tests

Mandatory scenarios:

1. OIDC login, MFA context, SCIM create/update/suspend/deprovision.
2. Tenant isolation across every CRUD, stream, search, export, cache, object,
   evidence, and analytics route.
3. Agent provision/update/suspend/quarantine/delete sagas with injected failures.
4. Durable task through planning, multiple actions, checkpoint, result, and cost.
5. AGS allow, deny, approval, handoff, outage, malformed, stale, revoked paths.
6. PUL real read and approval-gated write with scoped connection and receipt.
7. SPACE real Firecracker launch, governed command, browser action, artifacts,
   network enforcement, snapshot/cleanup, and verified termination.
8. GovAgn/evidence correlation from employee through action outcome and cost.
9. Scheduled and channel-initiated work using distinct workload identity.
10. Delegation with bounded authority, budget, depth, cancellation propagation.
11. Memory ACL, provenance, review, deletion, and index propagation.
12. Budget reservation, concurrent limit, settlement, failure release, hard stop.

- **TST-E2E-001:** End-to-end tests MUST assert the external effect and provider
  state, not only an HTTP 200 or UI message.
- **TST-E2E-002:** Every successful scenario MUST assert a complete evidence
  graph and verify artifact/receipt hashes.
- **TST-E2E-003:** Every denied scenario MUST assert that no downstream executor
  or provider received the action.

## 5. Security verification

### 5.1 Application/API

Test:

- authentication bypass, token confusion, issuer/audience/algorithm mistakes;
- CSRF, CORS, XSS, SSRF, open redirect, request smuggling/splitting;
- SQL/NoSQL/command/template/log injection, unsafe deserialization;
- IDOR/BOLA, privilege escalation, mass assignment, role/self update;
- pagination, search, export, WebSocket/stream authorization;
- body/result decompression and resource exhaustion;
- secret/error/debug leakage and unsafe default startup.

### 5.2 Agent/action abuse

Test prompt injection and confused-deputy attempts through:

- user prompts and attachments;
- websites, redirects, downloads, forms, hidden text, and browser snapshots;
- PUL connector results and messages;
- memory and enterprise knowledge;
- skill/tool descriptions and tool output;
- channel messages and attachments;
- delegated agents and schedules;
- model fallback and error messages.

Assertions:

- no new tool or permission appears;
- resolved effect/target cannot be relabelled by the model;
- all supported effects reach the Action Gateway and AGS;
- denied/approval-required actions cannot be executed via another tool class;
- secrets do not enter prompts, outputs, screenshots, logs, traces, or evidence;
- unsupported completion claims are rejected by evidence checks.

### 5.3 Approval and policy

Test:

- approval replay, reuse, theft, expiry, revocation, and digest substitution;
- target/resource change between approval and execution;
- requester self-approval and colluding role paths;
- stale policy, policy rollback, shadow mode, broken allow/deny rule;
- long-running action revoked mid-flight;
- break-glass initiation, dual approval, expiry, alert, and closure.

### 5.4 Isolation and egress

Test on every supported SPACE/kernel/runtime combination:

- microVM escape attempts and device/filesystem boundaries;
- host/peer/tenant discovery and access;
- metadata, loopback, link-local, private networks, raw sockets, DNS rebinding,
  redirect chains, alternate protocols, IPv4/IPv6, proxy bypass;
- CPU/memory/disk/process/file-descriptor/network exhaustion;
- malicious images, browser extensions, files, packages, and persistence;
- teardown data remanence and snapshot isolation.

### 5.5 Supply chain

Test:

- modified bundle/image/chart/model/skill/tool/package/signature/checksum;
- unmanifested extra content and omitted content;
- revoked/expired signing keys;
- vulnerable or forbidden licence artifact admission;
- downgrade below security minimum;
- compromised internal mirror and stale advisory feed;
- build provenance verification and source-to-artifact trace.

- **TST-SEC-001:** Independent penetration testing MUST use the signed release
  candidate and production-equivalent topology.
- **TST-SEC-002:** Critical/high findings block GA unless the CISO approves a
  documented time-bounded exception with compensating controls.
- **TST-SEC-003:** A planner-worker compromise exercise MUST prove the attacker
  cannot obtain connector/admin secrets, Docker/KVM/SPACE management, policy
  authoring, release signing, or evidence rewriting authority.

## 6. Agent quality and safety evaluations

Evaluation suites MUST be versioned datasets with reproducible model, prompt,
skill, tool, policy, and environment manifests.

Dimensions:

- task success and result correctness;
- appropriate tool selection and action schema validity;
- evidence-grounded completion;
- refusal/deny compliance and safe clarification;
- approval precision and comprehensible rationale;
- prompt-injection resistance and data exfiltration prevention;
- budget/step/time efficiency;
- recovery after tool/model/transient failure;
- memory relevance, provenance, and non-contamination;
- delegation quality and authority containment;
- schedule/channel delivery correctness;
- regression by model/tool/skill/policy version.

- **TST-EVL-001:** High-risk task categories MUST define minimum pass and maximum
  unsafe-action rates approved by the risk owner.
- **TST-EVL-002:** Zero policy-bypass tolerance applies to deterministic control
  tests regardless of average quality score.
- **TST-EVL-003:** Model or prompt changes require targeted regression plus the
  core safety/evidence suite before catalogue promotion.
- **TST-EVL-004:** Evaluation examples MUST avoid training/test leakage where
  practical and include adversarial holdout sets.
- **TST-EVL-005:** Human grading must record rubric, grader identity/qualification,
  inter-rater agreement where relevant, and protected-data controls.

## 7. Resilience and failure injection

Inject failures at every state transition:

- API/worker process kill, node drain, clock skew, network partition, DNS/NTP
  loss, certificate expiry;
- PostgreSQL primary failover, slow query, pool exhaustion, transaction conflict;
- event bus unavailable, delayed, duplicate, reordered, poison message, DLQ;
- object/evidence store unavailable, partial write, corrupt object, anchor delay;
- AGS/PUL/SPACE/GovAgn/model unavailable, timeout, malformed response, stale
  response, duplicate response;
- PUL/SPACE effect succeeds but response is lost;
- approval arrives during restart or after expiry/revocation;
- cancellation/quarantine/emergency stop during every action phase;
- disk full, quota exhaustion, model capacity exhaustion, SPACE pool exhaustion.

Required assertions:

- acknowledged user input and state are not lost;
- no blind duplicate irreversible effect occurs;
- uncertainty is visible and reconciled;
- evidence remains complete or the action fails closed;
- unrelated tenants continue within isolation/capacity limits;
- alerts and runbooks activate with correct severity;
- recovery converges without manual database mutation.

- **TST-RES-001:** Chaos tests MUST run repeatedly for a sustained window, not
  as one-off scripted restarts.
- **TST-RES-002:** Every discovered unsafe retry/unknown path gains a permanent
  regression test.

## 8. Performance and scale

Test stages:

1. Component microbenchmarks for digest, policy validation, state transition,
   evidence append, and serialization.
2. Reference load at expected tenant/task/action mix.
3. Stress to documented admission limits.
4. Soak for memory/resource leak and queue/retention behaviour.
5. Burst for schedules, approvals, model results, and computer startups.
6. Noisy-neighbour with one abusive tenant.
7. Failover under reference load.

Metrics:

- API latency/error/throughput;
- task queue/start/completion and time in each state;
- decision, approval notification, action dispatch, computer startup;
- model tokens/concurrency and connector rates;
- evidence append/verification/anchor lag;
- database/event/object resource and backlog;
- per-tenant fairness and quota accuracy;
- cost per task/action and capacity headroom.

- **TST-PERF-001:** The reference workload and dataset MUST be published with the
  release evidence.
- **TST-PERF-002:** SLOs MUST pass with at least 30% documented headroom on the
  reference hardware profile or an approved capacity model.
- **TST-PERF-003:** Overload MUST produce bounded queues, fair throttling, and
  stable rejection reasons rather than cascading failure.
- **TST-PERF-004:** Performance results from the Windows/WSL development profile
  do not qualify as production sizing evidence.

## 9. Data, backup, and restore tests

Test:

- schema migration from every supported source version;
- tenant RLS under owner/non-owner/migration/support roles;
- encryption/key rotation and mixed-key data;
- retention, legal hold, deletion, restore-after-delete protections;
- object manifest and database reference reconciliation;
- price version/cost reproduction;
- backup during live writes and point-in-time recovery;
- evidence chain/anchor verification before and after restore;
- in-flight action/task reconciliation after restore;
- total site loss and isolated recovery.

- **TST-DR-001:** Quarterly restore and annual full DR tests MUST use production
  procedures and independently verify RPO/RTO.
- **TST-DR-002:** A backup is not considered successful until a scheduled sample
  restore and integrity verification succeeds.

## 10. Install, upgrade, rollback, and air-gap tests

Test matrix includes:

- fresh install for every supported platform profile;
- invalid prerequisites and secure fail-fast;
- idempotent rerun and interrupted install;
- N-1 to N rolling upgrade and declared older upgrade paths;
- mixed-version compatibility and unsupported combination rejection;
- migration failure before/after canary;
- rollback before/after reversible boundary;
- client/server version skew;
- certificate, signing, encryption, and provider credential rotation;
- uninstall with preserve/export/delete choices;
- disconnected bundle verification/import/install/operation/diagnostics;
- attempts to reach public DNS, registries, models, telemetry, licence, or docs.

- **TST-REL-001:** Tests MUST operate on the exact release candidate bundle and
  record all artifact digests.
- **TST-REL-002:** The GA air-gap test MUST block external networking and run for
  a representative sustained period, including restart and upgrade.
- **TST-REL-003:** Rollback MUST preserve task/evidence integrity and reconcile
  in-flight work.

## 11. Accessibility and usability acceptance

Critical employee and admin journeys MUST be tested for keyboard operation,
screen-reader semantics, focus, contrast, responsive layouts, localization-safe
content, timeouts, error recovery, and approval comprehension.

- **TST-UX-001:** Deny, approval, unknown, partial, and failed states MUST not be
  represented only by colour.
- **TST-UX-002:** Approval usability testing MUST show users understand target,
  effect, destination, data, and consequence before deciding.
- **TST-UX-003:** Destructive actions require clear consequence and recovery
  information without habituating users to generic confirmation prompts.
- **TST-UX-004:** Product documentation MUST match the exact release and have
  offline availability.

## 12. Release evidence pack

The immutable evidence pack contains:

- release manifest/BOM and all artifact digests/signatures;
- source/build/provenance/SBOM/licence/vulnerability reports;
- environment and configuration manifest;
- requirement-to-test traceability matrix;
- unit/contract/integration/E2E/security/eval/performance/chaos results;
- real AGS/PUL/SPACE/GovAgn/model proof identifiers;
- live connector before/after state and receipt;
- tenant isolation and bypass-suite reports;
- evidence chain/anchor/export/restore verification;
- backup/restore/DR and RPO/RTO report;
- install/upgrade/rollback/air-gap acceptance;
- accessibility/usability assessment;
- penetration test and remediation report;
- known limitations, residual risk, waivers with expiry/owner;
- signed approvals by accountable authorities.

- **TST-EVP-001:** Raw logs and protected data MUST be minimized/redacted; the
  pack still provides cryptographic references to source evidence.
- **TST-EVP-002:** The pack MUST be independently verifiable with tools included
  in the offline bundle.
- **TST-EVP-003:** A failed or missing mandatory result cannot be replaced by a
  narrative assertion.

## 13. GA exit criteria

Enterprise GA requires all of the following:

1. Mandatory requirement traceability is complete with no unapproved gaps.
2. All P0/P1 correctness, security, tenancy, evidence, durability, install, and
   recovery defects are closed.
3. Independent penetration and architecture reviews are complete.
4. Real AGS policy, PUL live connector, SPACE Firecracker/browser, GovAgn
   evidence, model, approval, and cost flows pass end to end.
5. Reference load, soak, noisy-neighbour, and failover meet SLO/capacity.
6. Backup/restore and DR meet RPO/RTO with valid evidence chains.
7. Signed install/upgrade/rollback passes for supported profiles.
8. Air-Gap edition passes disconnected acceptance with no hidden dependencies.
9. On-call, alerts, dashboards, runbooks, support lifecycle, and escalation are
   operational and exercised.
10. Product, Architecture, CISO, SRE, and Independent Acceptance sign the exact
    release candidate evidence pack.

The following automatically block GA:

- known governance bypass or cross-tenant access;
- false success or blind duplicate irreversible effect;
- missing evidence for an acknowledged effect;
- default/embedded credentials or planner access to administrative secrets;
- production dependency on the local unauthenticated SQLite runtime;
- unverified/moving release artifacts;
- untested restore or upgrade path;
- a dry-run or mock presented as real connector/computer evidence.

