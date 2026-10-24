# Enterprise-Controlled Agent Platform

## Product intent

AgentFabric should become an enterprise-operated system for persistent AI
teammates. An employee signs in with an organisation account, creates one or
more agents, assigns approved models, skills, tools, software, data connections,
budgets, and computers, and then interacts with those agents through approved
channels.

The CIO owns service availability, capacity, approved technology, cost, and the
employee experience. The CISO owns trust policy, data boundaries, approvals,
evidence, retention, and incident controls. Employees operate inside those
guardrails; they cannot bypass them by choosing a different tool or asking an
agent to use a browser instead of an API.

The target is suitable for connected enterprise, sovereign cloud, on-premise,
disconnected, and fully air-gapped deployments.

## Non-negotiable control rule

An agent can **propose** an action but cannot directly exercise authority.

Every external side effect and every sensitive read must pass through one
governed action gateway. This includes:

- browser navigation, downloads, uploads, form entry, clicks, and submissions;
- shell commands, code execution, file reads, file writes, and package installs;
- MCP and API operations;
- email, chat, ticketing, source-control, ERP, CRM, and database operations;
- model calls involving classified or regulated data;
- creating, changing, delegating, scheduling, or deleting another agent;
- requesting a new computer, software package, secret, network destination, or
  privilege.

Model reasoning and ordinary conversation do not require a human approval for
every token. They remain traced and subject to data-handling policy. The action
gateway controls the point where information is read from a protected source or
an effect can occur.

## Separation of duties

No single runtime component should hold policy authority, enterprise
credentials, and execution power at the same time.

| Responsibility | Owning component | Must not own |
|---|---|---|
| Employee identity, tenant, role, and session | Enterprise identity gateway | Tool credentials or sandbox control |
| Agent identity, team membership, task state | Agent control plane | Provider administrator credentials |
| Policy decision | AGS Sovereign | Execution access or business credentials |
| Enterprise application capability | PUL | Model planning or infrastructure administrator access |
| Computer lifecycle and command execution | SPACE execution broker | Policy authoring or employee sessions |
| Evidence, trace, cost, and audit correlation | GovAgn and AgentFabric | Permission to approve its own actions |
| Planning and conversation | Agent runtime | Direct network, Docker, administrator tokens, or long-lived provider secrets |

This separation prevents a compromised planner or web application from turning
one vulnerability into full enterprise control.

## Target architecture

```text
                                  CIO / CISO administration
                    policies · catalogues · budgets · evidence · incidents
                                             |
                                             v
+----------------------+        +------------------------------+
| Enterprise identity  |------->| AgentFabric control plane    |
| OIDC · SSO · MFA      |        | tenants · users · agents     |
| SCIM · groups · roles |        | teams · entitlements · quota |
+----------+-----------+        +---------------+--------------+
           |                                            |
           | employee session                           | durable task
           v                                            v
+----------------------+        +------------------------------+
| Employee workspace   |<------>| Durable agent coordinator    |
| roster · chat · tasks |        | queue · lease · checkpoint   |
| approvals · takeover  |        | retry · schedule · cancel    |
+----------------------+        +---------------+--------------+
                                                    proposal |
                                                             v
                                   +-------------------------+
                                   | Governed Action Gateway |
                                   | canonical action envelope|
                                   | no direct execution       |
                                   +------+----------+---------+
                                          |          |
                                policy    |          | evidence
                                          v          v
                                +---------+--+   +---+----------------+
                                | AGS        |   | GovAgn/AgentFabric |
                                | decide     |   | trace · cost · SIEM|
                                | allow/deny |   | audit · reports    |
                                | approval   |   +--------------------+
                                +------+-----+
                                       | permit
                    +------------------+------------------+
                    |                                     |
                    v                                     v
          +--------------------+                +---------------------+
          | PUL capability     |                | SPACE execution     |
          | broker             |                | broker              |
          | app identity       |                | Firecracker lifecycle|
          | OAuth · MCP · API  |                | commands · browser  |
          | receipt · approval |                | files · software    |
          +---------+----------+                +----------+----------+
                    |                                      |
                    v                                      v
          Enterprise applications                Isolated agent computer
          and protected data                     approved image and egress
```

## Component responsibilities

### 1. Enterprise identity gateway

- Authenticate employees and administrators through enterprise OIDC/SAML and
  require MFA according to organisation policy.
- Synchronise employees, groups, departments, and employment status through
  SCIM or an offline equivalent.
- Issue short-lived sessions containing tenant, employee, role, group,
  assurance level, and device context.
- Enforce separate administrator, policy-author, approver, auditor, and employee
  roles.
- Revoke every derived agent session and capability when an employee is
  disabled.

The existing API gateway contains useful OIDC, RBAC, tenant, and budget
foundations. It must not be reused as the runtime identity boundary until the
known authentication-test, tenant-header, cookie, role-normalisation, and audit
gaps are remediated and independently tested.

### 2. AgentFabric control plane

- Maintain tenants, employees, agent teams, templates, ownership, membership,
  budgets, quotas, and lifecycle state.
- Convert CIO-approved model, tool, skill, software, and computer catalogues
  into employee entitlements.
- Create an immutable agent identity and derive its AGS identity, PUL
  application identity, evidence namespace, and SPACE execution scope.
- Prevent an employee from granting an agent privileges the employee does not
  possess.
- Support suspend, revoke, archive, export, and legal-hold operations.

Agent creation is a durable provisioning workflow, not a sequence of network
calls inside an HTTP request. Partial provisioning must be visible and
reconciled rather than retried blindly.

### 3. Durable agent coordinator

- Persist every task before dispatch.
- Use worker leases and checkpoints so tasks survive restarts and deployments.
- Represent `queued`, `running`, `waiting_for_policy`,
  `waiting_for_approval`, `waiting_for_employee`, `completed`, `failed`,
  `cancelled`, and `needs_reconciliation` states explicitly.
- Support schedules, deadlines, priorities, quotas, interruption, replay, and
  operator takeover.
- Store model-independent conversations and typed tool results.
- Load only approved, versioned skills and memory scopes.

The coordinator should own workflow state. It should not contain policy logic,
provider secrets, Docker access, or direct SPACE administration.

### 4. Governed Action Gateway

Every tool adapter implements one contract and produces one evidence shape.
The gateway is the only route from a planner to an effectful system.

```json
{
  "action_id": "act_...",
  "tenant_id": "tenant_...",
  "employee_id": "user_...",
  "agent_id": "agent_...",
  "task_id": "task_...",
  "session_id": "session_...",
  "action_type": "browser.form.submit",
  "target": "https://internal.example/orders/123",
  "input_digest": "sha256:...",
  "data_classification": ["internal"],
  "requested_capability": "orders.update",
  "side_effect": "reversible",
  "reason": "Update the delivery date requested by the employee",
  "idempotency_key": "...",
  "expires_at": "..."
}
```

The envelope contains safe metadata and cryptographic digests. Secrets, raw
prompts, and unnecessary customer data are not sent to the policy service.

The gateway must:

1. authenticate the caller and bind it to the employee, agent, and task;
2. validate the action against a typed schema and capability catalogue;
3. classify data, target, side effect, and risk;
4. request an AGS decision bound to the exact action digest;
5. pause for an authorised human approval when required;
6. call PUL or the SPACE execution broker with a short-lived permit;
7. record result digests and provider receipts;
8. emit a correlated GovAgn/AgentFabric event;
9. return a typed result to the durable coordinator.

No browser, model adapter, skill, MCP server, or agent plugin may bypass this
path.

### 5. AGS Sovereign: policy decision and approval authority

Use AGS as the enterprise policy decision point:

- bind decisions to tenant, employee, agent, task, action type, target,
  capability, data classification, and exact action digest;
- return `allow`, `deny`, `approval_required`, or `escalate` with a reason,
  policy revision, expiry, and decision identifier;
- require a distinct approver identity for high-risk operations;
- support CIO catalogues and CISO policy sets without embedding either in agent
  prompts;
- fail closed on timeout, malformed response, stale policy, identity mismatch,
  or unavailable approval state;
- expose revocation and emergency-stop decisions to the coordinator and
  execution brokers.

AGS decides whether an action may happen. It does not perform the action.

### 6. PUL: capability and enterprise connection broker

Use PUL as the only application-facing credential and capability boundary:

- keep OAuth refresh tokens, API credentials, and connector secrets outside the
  agent and outside SPACE;
- issue per-agent or per-team application identities with explicit operations;
- map high-level operations such as `repo.issue.create` or
  `mail.message.send` to reviewed adapters;
- enforce input/output schemas, rate limits, scopes, data-loss controls,
  idempotency, and approval requirements;
- return receipts for denied, pending, approved, completed, and failed actions;
- support offline/internal connectors without requiring a public SaaS control
  plane.

PUL does not approve infrastructure commands. An AGS permit to run code is not
a PUL permit to change a business application.

### 7. SPACE execution broker: isolated computers

- Run outside the agent computer and hold the only SPACE management identity.
- Create computers from CIO-approved, signed, versioned images.
- Bind each computer to one tenant, agent, task/session, network policy, resource
  budget, and retention policy.
- Execute only commands whose immutable content and arguments match the AGS
  permit digest.
- Provide approved software through images or an internal package catalogue;
  agents cannot install arbitrary public packages in an air gap.
- Route browser traffic through policy-aware egress and credential injection so
  credentials never enter the sandbox.
- Reconcile ambiguous create, command, snapshot, and delete operations without
  issuing a duplicate effect.
- Support immediate quarantine, snapshot, evidence preservation, and teardown.

The existing `SpaceComputerProvider`, `MissionBroker`, and journal provide a
good fail-closed foundation. Production state must move from local SQLite to a
transactional shared store with a durable outbox and externally anchored
evidence.

### 8. GovAgn and AgentFabric: evidence, operations, and FinOps

Every component emits events using the same correlation identifiers:

- tenant, employee, agent, team, task, session, action, policy decision, PUL
  receipt, SPACE sandbox/process, model call, trace, and evidence object;
- timestamps from a trusted enterprise time source;
- input and output digests with classification and redaction metadata;
- policy revision, model revision, skill revision, image digest, and software
  catalogue revision;
- token, compute, storage, connector, and licence cost;
- approval actor and separation-of-duty evidence.

GovAgn supplies the live operational trace and evidence stream. AgentFabric
adds tenant operations, budgets, audit verification, dashboards, alerts,
retention, reports, and SIEM/export interfaces.

Evidence storage must be append-only for writers, encrypted, retention-aware,
tenant-scoped, and periodically anchored outside the operational database.

## CIO and CISO operating model

### CIO controls

- approved models and routing policy;
- approved computer sizes, images, software, and capacity pools;
- approved tools, skills, connectors, and channels;
- tenant and department budgets, quotas, availability, and support levels;
- deployment rings, upgrades, backup, recovery, and business continuity;
- employee self-service limits and service ownership.

### CISO controls

- data classification and residency;
- identity assurance, least privilege, and separation of duties;
- network destinations and browser policy;
- allowed, denied, approval-required, and escalated action classes;
- secret handling, DLP, malware scanning, and software provenance;
- evidence retention, legal hold, audit export, and SIEM integration;
- incident quarantine, global kill switch, agent revocation, and forensic access.

### Employee experience

An employee should be able to:

1. sign in with the enterprise account;
2. create an agent from an approved role template;
3. choose only entitled models, skills, tools, software, and data connections;
4. start a private or approved team computer;
5. assign a task and watch structured progress;
6. respond to clarification or approval requests;
7. interrupt, take over, resume, or stop the agent;
8. inspect what the agent did, why it was allowed, what it changed, and what it
   cost;
9. share an agent with an approved team without sharing personal credentials;
10. archive or delete it subject to retention policy.

## Air-gapped deployment profile

Core operation must have no dependency on a public control plane.

Required local services include:

- enterprise identity provider, PKI, DNS, NTP, secrets manager, and SIEM;
- AgentFabric control plane, durable coordinator, action gateway, GovAgn, AGS,
  PUL, and SPACE;
- local model serving and an approved model catalogue;
- internal OCI, OS, Python, npm, and model registries or mirrors;
- signed agent images, software bundles, skills, policies, and connector
  packages;
- local object storage and databases with backup and disaster recovery;
- a controlled import/export station that verifies signatures, malware scans,
  manifests, provenance, and approvals;
- offline licence and entitlement handling when commercial licensing applies.

Deployment modes should share the same APIs and evidence model:

1. local development on Windows/WSL2;
2. connected enterprise Kubernetes;
3. sovereign private cloud;
4. disconnected on-premise Kubernetes;
5. fully air-gapped appliance or cluster.

Cloud connectors and remote sandboxes are optional adapters, never core
dependencies.

## Mapping to the current codebase

| Current asset | Decision |
|---|---|
| `integrations/space_provider/space_client.py` | Keep the strict transport contract; make credentials short-lived and broker-owned |
| `integrations/space_provider/mission_broker.py` | Keep fail-closed and reconciliation semantics; move state to shared transactional storage |
| `integrations/space_provider/ags_authorizer.py` | Keep as the initial AGS adapter; expand the decision context and signed permit contract |
| `integrations/space_provider/pul_client.py` | Keep as the initial PUL adapter; use it for every runtime operation instead of generic HTTP |
| `enterprise-agent/app.py` | Split into API, identity, agent registry, task coordinator, action gateway, tools, evidence, and UI modules |
| `api-gateway/` | Reuse OIDC/RBAC/budget concepts after fixing and testing the known security defects |
| `collector/`, `af-core/`, `portal/` | Reuse as the observability and FinOps plane; unify correlation and audit schemas |
| `deploy/` | Extend into signed connected and air-gapped release bundles |

Delete or retire these paths from the production design:

- hard-coded tenant identity;
- unauthenticated agent and artifact APIs;
- direct Playwright execution outside the action gateway;
- generic privileged HTTP clients where strict adapters exist;
- administrator tokens and Docker control in the user-facing runtime;
- process-local locks and SQLite as distributed coordination;
- synchronous multi-step execution inside an HTTP request;
- mutable script paths authorised only by their command-line arguments.

## Production delivery sequence

### Gate 0: security baseline

- repair and test identity, tenant, cookie, RBAC, and audit defects;
- define canonical IDs, action envelope, decision, receipt, and evidence schemas;
- define threat model, trust boundaries, data classes, and abuse cases;
- prohibit every known direct execution bypass.

### Gate 1: modular control path

- extract the employee API and agent registry from the monolith;
- implement the governed action gateway;
- route SPACE commands, PUL operations, browser actions, and file operations
  through it;
- remove administrator credentials and Docker access from the agent runtime;
- prove allow, deny, approval, timeout, revocation, and evidence correlation.

### Gate 2: durable agent service

- introduce PostgreSQL-backed task state and a durable queue/outbox;
- implement leases, checkpoints, retries, cancellation, approval pause/resume,
  schedules, reconciliation, and orphan cleanup;
- add quotas, retention, and per-agent resource accounting.

### Gate 3: enterprise teammate product

- ship employee roster, templates, task UI, live progress, approval inbox, and
  operator takeover;
- add approved model, skill, software, tool, connector, and channel catalogues;
- add long-term memory and controlled agent delegation;
- integrate Slack, Teams, email, webhooks, and internal portals through the same
  governed action model.

### Gate 4: air-gapped release

- produce signed offline bundles, manifests, SBOMs, provenance, and update
  media;
- validate installation with no DNS or internet route;
- test backup/restore, disaster recovery, certificate rotation, model and image
  updates, SIEM export, and evidence verification;
- complete performance, red-team, tenant-isolation, recovery, and independent
  acceptance tests.

## Release acceptance invariants

The product is not production-ready until all of these are continuously proven:

- no employee can see or operate another tenant's agent, computer, evidence, or
  connection;
- no agent or sandbox can obtain a long-lived enterprise credential;
- no effectful tool path bypasses the action gateway and AGS;
- no business application action bypasses PUL;
- no permitted command executes content different from the authorised digest;
- a process or node failure cannot silently lose, duplicate, or repeat an
  action;
- approval requires an authorised human distinct from the requesting agent;
- revocation and emergency stop prevent new actions within a defined bound;
- every completed action has a decision, executor result, receipt where
  applicable, and correlated evidence;
- the entire core system installs, operates, upgrades, backs up, and restores
  without public-network access in the air-gapped profile.
