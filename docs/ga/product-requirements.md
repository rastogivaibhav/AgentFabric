# Enterprise GA Product Requirements

## 1. Product outcome

AgentFabric Enterprise provides organisation-controlled persistent AI
teammates. Employees receive accounts through enterprise identity, create or
receive approved agents, assign allowed models/tools/software/data, and run
interactive or scheduled work. CIO and CISO controls remain effective across
chat, browser, computer, connector, API, schedule, and delegated-agent paths.

The product is not a general-purpose remote shell, a consumer desktop agent, or
an ungoverned plugin host. It is an enterprise system of delegated, observable,
revocable authority.

## 2. Personas and responsibilities

| Persona | Goals | Mandatory boundaries |
|---|---|---|
| Employee | Create/use teammates, review progress, answer questions, approve eligible actions | May only use entitled catalogues and own/team resources |
| Team owner | Curate templates, team memory, schedules, connections, and budgets | Cannot weaken enterprise security floors |
| Approver | Review a resolved action, data, destination, and risk before granting one-time approval | Cannot approve own prohibited action where separation is required |
| CIO/platform admin | Operate service, catalogues, capacity, releases, quotas, and cost | Cannot silently change CISO policies or evidence |
| CISO/security admin | Define policy, approvals, classifications, egress, emergency response | Cannot alter business results or erase evidence |
| Auditor/compliance | Search, verify, export, and attest evidence | Read-only; all evidence access is itself audited |
| Operator/SRE | Maintain availability, recover failures, rotate infrastructure credentials | No access to decrypted business content by default |
| Developer/integrator | Build approved tools, skills, channels, models, and connectors | Packages are signed, tested, admitted, and least-privileged |

## 3. Product principles

- **Simple for employees:** create a teammate from approved choices without
  understanding infrastructure internals.
- **Explicit authority:** show what the agent can access, why, for how long, and
  who approved it.
- **Progress over mystery:** show queued, planning, waiting, executing, retrying,
  completed, failed, cancelled, and uncertain states.
- **Safe interruption:** employees and operators can pause, steer, cancel, or
  take over without corrupting durable work.
- **Evidence by construction:** actions produce evidence as part of execution,
  not as a later logging best effort.
- **Sovereign by default:** core operation does not depend on a vendor public
  control plane.

## 4. Scope for Enterprise GA

### In scope

- Enterprise OIDC, MFA enforcement, SCIM lifecycle, groups, RBAC/ABAC.
- Multi-tenant organisations, employees, teams, and data boundaries.
- Agent definitions, instances, templates, cloning, versioning, suspension, and
  deletion.
- Durable interactive tasks, scheduled tasks, approvals, retries,
  cancellation, interruption, and recovery.
- Approved model, skill, tool, software, memory, connection, computer, and
  channel catalogues.
- Governed file, shell, browser, package, MCP, connector, model, schedule,
  memory-write, message-send, and delegation actions.
- SPACE computers and browsers; PUL enterprise connections; AGS policy and
  approval; GovAgn/AgentFabric evidence, observability, cost, and fleet views.
- Web employee workspace and separate administrative/auditor experiences.
- API and webhook integration for approved enterprise clients.
- Connected, sovereign/on-premises, and fully air-gapped deployment.
- Signed installation, upgrade, rollback, backup, restore, and diagnostics.

### Not required for initial GA

- Public consumer SaaS tenancy.
- Consumer app stores or unmanaged mobile distribution.
- A public marketplace allowing unreviewed third-party code.
- Employee-supplied remote MCP servers or arbitrary container images.
- Unrestricted host-computer control.
- Autonomous financial, legal, HR, safety-critical, or production changes
  without domain-specific policy and explicit enterprise enablement.
- Cross-region active/active control-plane writes.

## 5. Functional requirements

### 5.1 Identity, tenancy, and access

- **PRD-IDN-001:** The platform MUST authenticate employees and administrators
  using customer-managed OIDC and MUST support IdP-enforced MFA claims.
- **PRD-IDN-002:** The platform MUST support SCIM or an equivalent approved
  lifecycle API for create, update, suspend, reactivate, and deprovision.
- **PRD-IDN-003:** Every interactive and service request MUST resolve an
  organisation, tenant, subject, role/group set, authentication context, and
  correlation ID from verified identity.
- **PRD-IDN-004:** Tenant, team, and ownership boundaries MUST be enforced on
  every list, read, create, update, delete, export, stream, and search path.
- **PRD-IDN-005:** The UI MUST explain the employee's current organisation,
  roles, team context, and active delegated authority.
- **PRD-IDN-006:** Account suspension or group removal MUST revoke new task and
  action authority within five minutes and MUST interrupt affected active work
  according to policy.
- **PRD-IDN-007:** Local password login MAY exist for isolated recovery only; it
  MUST be disabled by default, separately governed, rate-limited, and audited.

Acceptance:

- Cross-tenant negative tests cover every API and stream.
- A disabled user cannot start, resume, approve, or observe work after the
  revocation objective.
- Employee self-service cannot change roles, groups, tenant, status, or policy.

### 5.2 Agent lifecycle and roster

- **PRD-AGT-001:** An entitled employee MUST be able to create an agent from an
  approved template and choose only entitled model, computer, skill, software,
  connection, memory, channel, schedule, retention, and budget profiles.
- **PRD-AGT-002:** An agent definition MUST have an owner, tenant, version,
  purpose, instructions, classification ceiling, risk tier, catalogue
  references, budget, retention policy, and lifecycle state.
- **PRD-AGT-003:** Agent changes MUST create immutable versions. Running tasks
  MUST retain the version used at start unless an emergency policy revokes it.
- **PRD-AGT-004:** Supported states MUST include draft, provisioning, active,
  suspended, quarantined, deleting, deleted, and provisioning-failed.
- **PRD-AGT-005:** Provisioning MUST be durable and idempotent across
  AgentFabric, AGS, PUL, SPACE, and GovAgn identities/resources.
- **PRD-AGT-006:** The employee roster MUST show owner, purpose, status, active
  work, computer state, connections, last activity, budget posture, and policy
  alerts without exposing secrets.
- **PRD-AGT-007:** Delete MUST offer policy-defined retention or cryptographic
  erasure of conversations, memory, workspaces, and artifacts while preserving
  legally required evidence.
- **PRD-AGT-008:** Administrators MUST be able to suspend or quarantine one
  agent without interrupting unrelated tenants or agents.

Acceptance:

- Repeated create/delete requests converge without duplicate identities or
  orphaned computers.
- Quarantine prevents new actions and revokes active capabilities while
  preserving evidence.

### 5.3 Conversation, tasks, and durable work

- **PRD-TSK-001:** Employees MUST be able to submit a task, attach approved
  inputs, receive streaming progress, answer questions, steer, pause, resume,
  cancel, and inspect final results.
- **PRD-TSK-002:** Task state MUST survive API, worker, node, and planned
  platform restarts without losing acknowledged user input.
- **PRD-TSK-003:** Task states MUST include queued, planning, awaiting-policy,
  awaiting-approval, awaiting-input, executing, retrying, reconciling,
  completed, failed, cancelled, and quarantined.
- **PRD-TSK-004:** Every transition MUST include actor, reason, timestamp,
  source state, destination state, version, and evidence reference.
- **PRD-TSK-005:** The system MUST distinguish safe retry, unsafe retry, and
  unknown outcome. Unknown effects MUST enter reconciliation rather than replay.
- **PRD-TSK-006:** Employees MUST see which actions are planned, running,
  waiting, completed, denied, failed, cancelled, or uncertain.
- **PRD-TSK-007:** Cancellation MUST stop future actions immediately and
  terminate or compensate active execution within the policy-defined objective.
- **PRD-TSK-008:** The coordinator MUST support bounded retries, deadlines,
  per-action timeouts, retry budgets, dead-letter handling, and operator replay.
- **PRD-TSK-009:** Results MUST identify source artifacts, unresolved questions,
  confidence/limitations where applicable, and the evidence chain.

Acceptance:

- A task requiring approval resumes after worker replacement.
- Forced timeouts and ambiguous connector/VM outcomes never produce duplicate
  irreversible effects.

### 5.4 Models and planning

- **PRD-MDL-001:** All model invocation MUST use the Model Gateway and an
  approved model profile; planners MUST NOT call model providers directly.
- **PRD-MDL-002:** Model profiles MUST define provider/runtime, model digest or
  immutable version, allowed classifications, region, context limits, cost,
  fallback policy, and logging/redaction rules.
- **PRD-MDL-003:** Air-gap operation MUST support internally hosted models with
  no required public licence or telemetry call at runtime.
- **PRD-MDL-004:** Model fallback MUST never cross a data-classification,
  residency, risk, or cost boundary.
- **PRD-MDL-005:** The planner MUST emit typed action proposals validated
  against versioned schemas; free-form text MUST NOT invoke a tool.
- **PRD-MDL-006:** Prompt and context assembly MUST record the template/version,
  memory references, skill versions, model profile, and redaction policy without
  necessarily storing protected plaintext.
- **PRD-MDL-007:** Token/cost capacity MUST be reserved before invocation and
  settled after completion.
- **PRD-MDL-008:** Model unavailability MUST degrade the affected profile and
  MUST NOT crash unrelated task workers.

### 5.5 Governed actions and human approval

- **PRD-ACT-001:** File, shell, browser, package, MCP, connector, model,
  schedule, channel, software, memory-write, export, and delegation operations
  MUST use one Governed Action Gateway.
- **PRD-ACT-002:** The product MUST show the resolved effect, target,
  destination, data class, requested scopes, cost/resource estimate, policy
  reason, and approval status for each action.
- **PRD-ACT-003:** AGS verdicts MUST include allow, deny, approval-required, and
  handoff-required. Unavailable or malformed policy responses MUST fail closed.
- **PRD-ACT-004:** A human approval MUST be bound to one action digest, one
  context, one approver decision, an expiry, and a one-time nonce.
- **PRD-ACT-005:** The gateway MUST re-resolve the target and re-evaluate current
  policy immediately before consuming approval.
- **PRD-ACT-006:** Employees MUST be able to view denial and approval reasons in
  comprehensible language, subject to security-safe redaction.
- **PRD-ACT-007:** Approval queues MUST support delegation, escalation,
  separation of duties, expiry, reminder, withdraw, deny, and handoff.
- **PRD-ACT-008:** No action may be represented as completed without a validated
  terminal receipt or an explicit `unknown` outcome.

### 5.6 Computers, browsers, and software

- **PRD-CMP-001:** Each active execution context MUST receive a SPACE computer
  or an explicitly approved equivalent profile with tenant/session isolation.
- **PRD-CMP-002:** The production browser MUST execute inside the governed SPACE
  boundary or a separately certified broker with equivalent network and
  evidence controls; the local Docker sidecar is not a GA provider.
- **PRD-CMP-003:** Computer profiles MUST define image/template digest, kernel,
  CPU, memory, storage, duration, network, software, snapshot, and cleanup
  policies.
- **PRD-CMP-004:** Browser navigation, redirects, DNS results, downloads,
  uploads, clipboard, authentication, clicks, typing, and submissions MUST be
  individually governed according to semantic effect.
- **PRD-CMP-005:** Software and package installation MUST use an approved
  internal catalogue, immutable artifacts, and malware/licence policy.
- **PRD-CMP-006:** Workspaces MUST support encrypted persistence, snapshot,
  restore, export, retention, and secure deletion according to profile.
- **PRD-CMP-007:** Users MUST see whether a computer is starting, ready, busy,
  suspended, reconciling, stopped, or failed and which execution profile it uses.
- **PRD-CMP-008:** Direct Docker, KVM, SPACE-management, or host-shell access
  MUST NOT be exposed to planners, employee clients, or channel adapters.

### 5.7 Enterprise applications and connections

- **PRD-CONN-001:** PUL MUST be the exclusive broker for enterprise application
  credentials, app identities, user delegation, token lifecycle, and connector
  receipts.
- **PRD-CONN-002:** Employees MUST be able to request and view connections but
  MUST never see stored provider secrets after entry.
- **PRD-CONN-003:** Connections MUST be scoped by tenant, owner/team, provider,
  operations, objects, data class, destination, schedule, expiry, and policy.
- **PRD-CONN-004:** Revocation MUST block new operations promptly and SHOULD
  cancel in-flight operations where the provider permits.
- **PRD-CONN-005:** Connector output MUST carry provider identity, operation,
  resolved target, receipt, time, status, data classification, and result hash.
- **PRD-CONN-006:** GA acceptance MUST include at least one real read and one
  real approval-gated write against a customer-controlled test application; dry
  run is insufficient.

### 5.8 Skills, tools, and controlled extensibility

- **PRD-SKL-001:** Skills MUST be versioned, signed, reviewable instruction
  packages and MUST NOT grant authority merely by being selected.
- **PRD-SKL-002:** Tool availability MUST be the intersection of enterprise
  catalogue admission, tenant entitlement, agent entitlement, runtime
  relevance, and per-invocation AGS decision.
- **PRD-SKL-003:** Tool and MCP packages MUST run out of the control-plane
  process with declared schemas, permissions, network destinations, resources,
  provenance, and isolation profile.
- **PRD-SKL-004:** Employees MUST NOT install arbitrary remote MCP endpoints,
  npm packages, images, skills, or plugins in the Enterprise GA profile.
- **PRD-SKL-005:** Catalogue promotion MUST include tests, security review,
  licence review, signature, owner, support tier, and rollback version.
- **PRD-SKL-006:** Skills MAY load progressively to reduce context, but their
  complete immutable version MUST be recorded in task evidence.

### 5.9 Memory and enterprise knowledge

- **PRD-MEM-001:** Memory MUST be explicitly scoped as employee, agent, team, or
  organisation and MUST carry source, classification, purpose, retention,
  owner, policy version, and provenance.
- **PRD-MEM-002:** Automatic durable memory writes MUST pass review policy and
  MUST be visible, correctable, and deletable when retention permits.
- **PRD-MEM-003:** Retrieval MUST enforce source ACLs and the invoking subject's
  current authority; indexing MUST NOT broaden access.
- **PRD-MEM-004:** The product MUST distinguish curated memory, task history,
  approved knowledge sources, and transient context.
- **PRD-MEM-005:** Memory export/deletion/legal hold MUST propagate to indexes
  and caches with verifiable completion.
- **PRD-MEM-006:** Agent-generated content MUST NOT become trusted enterprise
  knowledge without provenance and the configured review workflow.

### 5.10 Schedules, channels, and delivery

- **PRD-SCH-001:** Employees MUST be able to create schedules only within their
  agent, connection, cost, time, and policy entitlements.
- **PRD-SCH-002:** Schedules MUST define timezone, recurrence, concurrency,
  missed-run, retry, expiry, notification, and approval policy.
- **PRD-SCH-003:** A scheduled run MUST use a distinct workload identity and MAY
  have less authority than an attended run.
- **PRD-SCH-004:** The scheduler MUST prevent unapproved overlap and recover
  stale or missed runs deterministically.
- **PRD-CHN-001:** Web, API, webhook, Teams, Slack, and email adapters MUST enter
  the same server-side task runtime and MUST NOT implement separate agent loops.
- **PRD-CHN-002:** Channel identity MUST map to a verified enterprise subject or
  service identity before task creation.
- **PRD-CHN-003:** Outbound delivery MUST use a transactional outbox and expose
  not-started, sending, delivered, failed, and unknown states.
- **PRD-CHN-004:** Channel adapters MUST apply data classification, destination,
  DLP, attachment, retention, and impersonation policy.

### 5.11 Delegation and teams of agents

- **PRD-DEL-001:** An agent MAY delegate only to an approved agent/template and
  only within the parent's remaining authority, classification, time, and
  budget.
- **PRD-DEL-002:** Child tasks MUST have distinct identities, bounded depth,
  concurrency, budget, and expiry.
- **PRD-DEL-003:** Parent and child actions MUST be visible as one evidence graph
  without sharing credentials or mutable private memory by default.
- **PRD-DEL-004:** Cancellation, quarantine, or revocation MUST propagate through
  the active delegation graph according to policy.
- **PRD-DEL-005:** Cycles and uncontrolled recursive delegation MUST be rejected.

### 5.12 Administration, evidence, and FinOps

- **PRD-ADM-001:** Separate employee, team owner, CIO, CISO, operations, and
  auditor experiences MUST expose only role-appropriate controls.
- **PRD-ADM-002:** CIO controls MUST include capacity, catalogue, release,
  quotas, budget, availability, usage, and service health.
- **PRD-ADM-003:** CISO controls MUST include policy versions, approvals,
  classifications, connection/network rules, quarantine, revoke, emergency
  stop, and evidence completeness.
- **PRD-ADM-004:** Auditor controls MUST support read-only search, chain
  verification, scoped export, retention/hold status, and access audit.
- **PRD-EVD-001:** Every action MUST show proposal, normalized target/effect,
  policy decision, approval, execution, receipt, artifact hashes, cost, and
  final/unknown status in one evidence timeline.
- **PRD-FIN-001:** Budgets MUST support tenant, department/team, agent, task,
  model, capability, and schedule scopes with reservation and settlement.
- **PRD-FIN-002:** Cost records MUST be reproducible from an effective-dated
  price catalogue and immutable usage inputs.
- **PRD-FIN-003:** Hard limits MUST prevent new reservations; reporting-only
  telemetry limits MUST not be labelled hard controls.

## 6. Required user journeys

The following end-to-end journeys are release-blocking:

1. IdP-provisioned employee creates an agent from an approved template.
2. Agent provisions AGS, PUL, SPACE, and evidence identities idempotently.
3. Employee runs a read-only task and watches streaming, attributable progress.
4. Agent proposes a denied action and cannot bypass denial through browser,
   shell, model, MCP, channel, or delegation.
5. Agent proposes an approval-gated write; a distinct approver inspects the
   resolved target and data, approves once, and the action executes once.
6. Worker dies before, during, and after an external effect; the system resumes
   or reconciles without duplicate effects.
7. Scheduled task runs under a narrower workload identity and produces a
   channel delivery receipt.
8. Employee is suspended while work is active; permissions are revoked and the
   task is safely stopped or quarantined.
9. CISO quarantines an agent and later releases it under a new policy version.
10. Auditor reconstructs the complete action and verifies artifact/evidence
    integrity without platform-administrator privileges.
11. Operator restores control state and evidence indexes from backup and proves
    RPO/RTO and chain continuity.
12. Independent team installs, upgrades, rolls back, and diagnoses the air-gap
    edition with the network disconnected.

## 7. Product telemetry and success measures

GA telemetry MUST be customer-owned and configurable. It SHOULD measure:

- task success, user-cancel, deny, approval, failure, and unknown rates;
- time in queue, planning, approval, execution, and delivery;
- restart recovery and duplicate-effect prevention;
- agent adoption and repeat use without capturing protected content;
- policy coverage and bypass-test results;
- evidence completeness and verification failures;
- model/tool/connector/computer availability and cost;
- support incidents, mean time to detect, acknowledge, mitigate, and recover.

Success measures never override security gates. A high completion rate cannot
compensate for missing tenant isolation or evidence.

## 8. Product acceptance

Product acceptance requires:

- all mandatory journeys passing in connected and sovereign profiles;
- air-gap journeys passing for the Air-Gap edition;
- accessibility review of employee and administrative critical journeys;
- user documentation for every state, denial, approval, error, and recovery;
- no placeholder, dry-run, simulated, or mock result presented as a real effect;
- clear labelling of development-only providers and unsupported configurations;
- signed approval from Product, CIO delegate, CISO delegate, SRE, and independent
  acceptance against the verification specification.

