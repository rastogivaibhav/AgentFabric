# Enterprise GA Security, Governance, and Compliance Specification

## 1. Security objective

AgentFabric must let an enterprise delegate useful work to AI agents while
preserving human and organisational control over identity, data, credentials,
networks, software, cost, and external effects. The model and its generated
content are untrusted inputs. A sandbox reduces blast radius but does not grant
authority.

The security design follows zero trust, least privilege, separation of duties,
default deny, complete mediation, durable evidence, and recoverable failure.

## 2. Threat model

### Protected assets

- employee and service identities;
- prompts, conversations, memory, attachments, and workspace data;
- enterprise application credentials and delegated tokens;
- policy bundles, approvals, signing keys, and capability grants;
- models, weights, system prompts, skills, tools, software, and templates;
- computers, browsers, networks, internal services, and connected applications;
- action evidence, audit exports, telemetry, usage, budgets, and pricing;
- release artifacts, update authority, backup, and disaster-recovery material.

### Threat actors and failure sources

- malicious or compromised employee account;
- external attacker exploiting API, UI, connector, browser, dependency, or
  sandbox vulnerabilities;
- prompt injection or poisoned content instructing the model to bypass policy;
- compromised or malicious model, skill, tool, MCP server, plugin, package,
  browser content, or connector response;
- tenant attempting cross-tenant access;
- privileged insider or operator misuse;
- compromised service/workload identity;
- supply-chain tampering or rollback to vulnerable artifacts;
- infrastructure failure causing duplicate, lost, reordered, or ambiguous work;
- model hallucination represented as an executed or verified outcome;
- evidence writer, index, backup, or time source compromise.

### Security boundaries

1. Employee/client to identity and API edge.
2. API/control plane to planner workers.
3. Planner to Governed Action Gateway and Model Gateway.
4. Action Gateway to AGS, Approval, PUL, and SPACE.
5. SPACE sandbox to host, network, browser, and enterprise destinations.
6. PUL connector worker to provider and credential store.
7. Operational state to evidence/WORM and analytics projections.
8. Build/release environment to customer offline distribution.
9. Administrative plane to tenant/business data.

## 3. Identity and access management

- **SEC-IAM-001:** Human production access MUST use customer-approved SSO;
  privileged roles MUST require phishing-resistant MFA where the IdP supports it.
- **SEC-IAM-002:** SCIM deprovisioning, group removal, and emergency revocation
  MUST invalidate new authority within five minutes or a stricter customer SLA.
- **SEC-IAM-003:** Every service MUST use a unique workload identity; shared
  symmetric platform-wide service secrets are prohibited for GA.
- **SEC-IAM-004:** Workload credentials MUST be short-lived, automatically
  rotated, audience-bound, and delivered without embedding in images or source.
- **SEC-IAM-005:** Platform administrator, tenant administrator, security
  administrator, operator, approver, auditor, and employee roles MUST be
  distinct and composable through groups/attributes.
- **SEC-IAM-006:** Cross-tenant platform support access MUST require an explicit
  case/reason, elevated workflow, time-bound session, customer-visible evidence,
  and read-only default.
- **SEC-IAM-007:** Impersonation MUST be disabled by default. If enabled for
  support, the UI and evidence MUST continuously show the real and represented
  identities.
- **SEC-IAM-008:** Session management MUST include secure cookie/token storage,
  idle and absolute timeout, revocation, device/session listing, and privileged
  reauthentication.
- **SEC-IAM-009:** API keys, if supported, MUST be hashed at rest, scoped,
  expiring, rotatable, attributable, and prohibited from interactive admin use.
- **SEC-IAM-010:** Authorization decisions MUST use current membership and policy
  state; cached authorization has a bounded revocation window.

## 4. Tenant isolation

- **SEC-TEN-001:** Tenant identity MUST originate from authenticated membership
  or workload assignment and MUST NOT be accepted from a mutable request header,
  URL parameter, model output, or client-supplied object.
- **SEC-TEN-002:** PostgreSQL tenant-owned tables MUST use forced RLS under
  non-owner roles plus application-layer checks.
- **SEC-TEN-003:** Object keys, event topics/partitions, cache keys, search
  indexes, metrics labels, evidence chains, and backups MUST include validated
  tenant scope.
- **SEC-TEN-004:** Tenant cryptographic separation MUST be supported for
  customers requiring tenant-specific data-encryption keys.
- **SEC-TEN-005:** Global identifiers MUST not confer access and SHOULD be
  non-enumerable.
- **SEC-TEN-006:** Tenant deletion MUST discover and process replicas, indexes,
  caches, backups, artifacts, credentials, sandboxes, and scheduled work subject
  to legal hold and contracted retention.
- **SEC-TEN-007:** Automated cross-tenant negative testing MUST cover every API,
  stream, export, admin operation, event consumer, and object retrieval path.

## 5. Policy and authorization

- **SEC-POL-001:** AGS MUST be the authoritative pre-execution PDP for all
  sensitive reads and effects.
- **SEC-POL-002:** Enforcement points MAY add stricter immutable denials but MUST
  NOT convert AGS deny, approval-required, handoff-required, malformed, expired,
  or unavailable outcomes into allow.
- **SEC-POL-003:** Policy evaluation context MUST include verified actor,
  initiator, tenant, agent, task, effect, resolved target, data class, purpose,
  destination, credentials, software, network, resource/cost, content digests,
  and prior evidence.
- **SEC-POL-004:** Policies MUST be versioned, signed, testable, explainable,
  promoted through environments, and rollback-capable.
- **SEC-POL-005:** Production policy changes MUST require peer review and
  customer-configured separation of duties; emergency change uses break-glass.
- **SEC-POL-006:** Shadow evaluation MAY measure proposed policy changes but
  cannot disable baseline controls or grant execution authority.
- **SEC-POL-007:** Policy decisions MUST carry stable reason codes and safe
  explanations for employees, approvers, operators, and auditors.
- **SEC-POL-008:** Revocation MUST be checked before dispatch and at meaningful
  long-running execution checkpoints.
- **SEC-POL-009:** Permission inheritance and delegation MUST only narrow
  authority; unioning unrelated parent grants is prohibited.

## 6. Human approval and break-glass

- **SEC-APR-001:** Approval MUST be based on the trusted resolved action, not the
  model's description of the action.
- **SEC-APR-002:** Approval UI MUST show actor/agent, effect, target, arguments or
  safe summary, data class, destination, credential scope, resource/cost,
  irreversible consequences, policy reason, and expiry.
- **SEC-APR-003:** Approval is single-use, action-digest-bound, expiring, and
  invalidated by any security-relevant change.
- **SEC-APR-004:** Execution MUST re-resolve and re-authorize after approval.
- **SEC-APR-005:** Approval eligibility MUST be computed from current identity,
  training/role, separation, conflict, and scope rules.
- **SEC-APR-006:** Bulk approval MUST be disabled unless each member action is
  independently resolved and the policy explicitly permits bounded batching.
- **SEC-APR-007:** Break-glass MUST require dual authorization, explicit reason,
  bounded tenant/action/time scope, automatic expiry, immediate alerting, and
  immutable evidence.
- **SEC-APR-008:** Break-glass MUST NOT allow evidence deletion, release-signature
  bypass, or silent cross-tenant access.
- **SEC-APR-009:** There is no employee-accessible YOLO, approval-off, or
  unrestricted full-access mode.

## 7. Secrets and enterprise credentials

- **SEC-SEC-001:** Secrets MUST be stored in a customer-approved Vault/KMS/HSM or
  equivalent and represented to applications by opaque references.
- **SEC-SEC-002:** Root/administrator credentials MUST be confined to
  provisioning or emergency components and never used for routine runtime work.
- **SEC-SEC-003:** PUL MUST keep provider credentials and delegated refresh
  tokens out of model context, planner memory, task records, logs, traces,
  screenshots, and receipts.
- **SEC-SEC-004:** Runtime capabilities MUST be least-privileged, action-bound,
  short-lived, nonce-bearing, audience-bound, and non-exportable where possible.
- **SEC-SEC-005:** Secrets MUST be redacted at ingress, structured logging,
  exception, trace, event, artifact preview, and support-bundle boundaries.
- **SEC-SEC-006:** Secret rotation MUST not require full-platform downtime and
  MUST expose mixed/failed rotation state.
- **SEC-SEC-007:** Secret access and decryption MUST emit evidence identifying
  workload, purpose, secret reference, scope, and outcome without secret value.
- **SEC-SEC-008:** Development secrets/default passwords MUST cause production
  startup failure.

## 8. Planner, prompt injection, and model safety

- **SEC-LLM-001:** All model output, retrieved content, webpage content,
  connector content, files, and messages are untrusted until validated.
- **SEC-LLM-002:** Instructions from external content MUST NOT alter platform,
  tenant, agent, or task authority.
- **SEC-LLM-003:** Tools are exposed by typed schema and entitlement; prompt text
  cannot create a new capability.
- **SEC-LLM-004:** Trusted system/developer instructions and untrusted content
  MUST be structurally separated and tagged with provenance where supported.
- **SEC-LLM-005:** The planner MUST not receive long-lived credentials, raw
  approval tokens, policy authoring authority, or host-management interfaces.
- **SEC-LLM-006:** High-risk actions require deterministic control checks; an LLM
  reviewer MAY advise but MUST NOT make the final authorization decision.
- **SEC-LLM-007:** Completion claims MUST be checked against receipts and required
  evidence. Unsupported claims are returned as limitations, not success.
- **SEC-LLM-008:** Prompt-injection and tool-confusion evaluations MUST cover
  browser, files, connectors, memory, skills, channels, and delegated agents.

## 9. Tool, skill, MCP, and plugin security

- **SEC-EXT-001:** Every executable extension MUST have an owner, immutable
  version/digest, signature, provenance, SBOM, licence, vulnerability status,
  declared permissions, data classes, destinations, resource limits, and
  support tier.
- **SEC-EXT-002:** Untrusted extensions MUST run out of process in a sandbox;
  in-process plugins with control-plane privileges are prohibited.
- **SEC-EXT-003:** Skills contain instructions and metadata only; they cannot
  grant permissions, receive secrets implicitly, or modify themselves in GA.
- **SEC-EXT-004:** MCP servers MUST be admitted into the catalogue and connected
  through a broker. Employee-entered remote endpoints are prohibited.
- **SEC-EXT-005:** Tool schemas and output MUST be bounded and validated;
  oversized or malformed responses fail safely.
- **SEC-EXT-006:** Extension update and rollback follow the signed release
  catalogue; dynamic public package installation is prohibited.
- **SEC-EXT-007:** Revoked/vulnerable artifacts MUST be blocked for new work and
  discoverable in active/running agent inventories.

## 10. SPACE, browser, and network security

- **SEC-SPC-001:** SPACE worker nodes MUST support hardware virtualization and
  a supported kernel; degraded isolation cannot silently fall back to host or a
  weaker provider.
- **SEC-SPC-002:** Each computer MUST have isolated compute, memory, process,
  filesystem, IPC, device, and network boundaries appropriate to its profile.
- **SEC-SPC-003:** Sandbox templates MUST be minimal, signed, scanned,
  reproducible, digest-pinned, and free of embedded secrets.
- **SEC-SPC-004:** Default egress is deny. Allowed destinations are resolved and
  enforced at the network layer with DNS rebinding, redirect, metadata,
  loopback, link-local, private-range, and protocol controls.
- **SEC-SPC-005:** Browser downloads and uploads MUST pass content type, size,
  malware, classification, and destination controls.
- **SEC-SPC-006:** Browser profiles, cookies, clipboard, storage, and sessions
  MUST be isolated per approved scope and destroyed or retained by policy.
- **SEC-SPC-007:** `--no-sandbox`, host networking, broad device mounts,
  privileged mode, and unrestricted host volumes are prohibited for GA
  execution workloads.
- **SEC-SPC-008:** Resource limits MUST protect platform availability from CPU,
  memory, storage, process, file descriptor, and network exhaustion.
- **SEC-SPC-009:** Runtime integrity, escape, or policy-enforcement failure MUST
  terminate/quarantine affected workloads and alert the security plane.
- **SEC-SPC-010:** Command/script authorization MUST bind immutable content,
  interpreter/tool, arguments, environment, working directory, image, software,
  user, and network policy.

## 11. PUL and connector security

- **SEC-PUL-001:** Connections MUST be least-privileged and separately scoped
  for read, create, update, send, delete, administer, and credential operations.
- **SEC-PUL-002:** Provider OAuth consent and delegated user identity MUST be
  visible and revocable by the user/administrator according to policy.
- **SEC-PUL-003:** Provider responses are untrusted content and pass schema,
  size, classification, malware/DLP, and prompt-injection handling.
- **SEC-PUL-004:** Connector calls MUST enforce resolved target and operation
  independently from the planner and Action Gateway.
- **SEC-PUL-005:** Irreversible provider operations require an idempotency or
  reconciliation strategy documented in the connector contract.
- **SEC-PUL-006:** Connector SDKs run with no access to other provider secrets,
  tenants, or general platform databases.
- **SEC-PUL-007:** PUL administrator APIs and runtime operation APIs MUST use
  separate endpoints/identities and network policies.

## 12. Data protection and privacy

- **SEC-DAT-001:** All network traffic MUST use authenticated encryption; all
  persistent stores and backups MUST use approved encryption at rest.
- **SEC-DAT-002:** Data classification and purpose MUST propagate through task,
  memory, model, action, connector, artifact, evidence, telemetry, and export.
- **SEC-DAT-003:** Model/connector/region selection MUST enforce classification,
  residency, consent, and contractual restrictions.
- **SEC-DAT-004:** Logs and metrics MUST minimize content and identifiers;
  protected content is off by default and separately governed.
- **SEC-DAT-005:** Retention, legal hold, export, deletion, rectification, and
  cryptographic erasure MUST be supported with durable jobs and evidence.
- **SEC-DAT-006:** Search/vector indexes and caches MUST inherit source ACL,
  classification, retention, and deletion.
- **SEC-DAT-007:** Support bundles MUST be previewable, scoped, redacted,
  encrypted, expiring, and customer-controlled.
- **SEC-DAT-008:** Synthetic or approved data MUST be used in standard support,
  demos, performance tests, and development environments.

## 13. Evidence and audit security

- **EVD-001:** Evidence MUST record proposal before effect, normalized target,
  policy/approval, dispatch, executor/provider receipt, artifacts, cost, and
  final or unknown outcome.
- **EVD-002:** Evidence records MUST be append-only and integrity-protected by
  per-chain hashes/signatures that support multiple writers safely.
- **EVD-003:** Chains MUST be externally anchored to customer-controlled WORM,
  transparency, timestamp, or equivalent storage on a defined interval.
- **EVD-004:** Evidence time MUST use authenticated enterprise time sources and
  record both occurred and received timestamps.
- **EVD-005:** Evidence payloads MUST be minimized and encrypted while retaining
  sufficient metadata and digests for verification.
- **EVD-006:** Access, search, verify, export, retention change, legal hold, and
  key operations on evidence MUST themselves be audited.
- **EVD-007:** Analytics loss, sampling, or reprocessing MUST not affect
  authoritative evidence validity.
- **EVD-008:** Restore MUST preserve chain identity, anchors, sequence, and
  verifier compatibility.
- **EVD-009:** No normal or break-glass role may update/delete evidence. Retention
  expiry produces a signed tombstone/manifest under controlled destruction.
- **EVD-010:** Evidence completeness monitors MUST detect missing expected
  proposal/decision/receipt/outcome relationships.

## 14. Supply-chain security

- **SEC-SUP-001:** Builds MUST be reproducible where practical and occur in
  isolated, ephemeral, least-privileged builders.
- **SEC-SUP-002:** Source, dependencies, build tools, base images, models,
  browsers, skills, tools, templates, and release outputs MUST be pinned and
  integrity-verified.
- **SEC-SUP-003:** Every shipped executable artifact MUST have an SBOM,
  provenance attestation, signature, checksum, licence inventory, vulnerability
  status, and source/build reference.
- **SEC-SUP-004:** Release signing keys MUST use HSM-backed or equivalently
  protected custody with separation from build workers.
- **SEC-SUP-005:** The offline bundle MUST be verified before import and again
  before deployment; failed verification blocks install.
- **SEC-SUP-006:** Dependency and vulnerability exceptions require owner,
  exploitability analysis, compensating control, expiry, and customer-visible
  release documentation where material.
- **SEC-SUP-007:** Production images MUST run read-only where possible, as
  non-root, with dropped capabilities, seccomp/AppArmor/SELinux policy, bounded
  resources, and no shell/package manager unless required.
- **SEC-SUP-008:** Rollback artifacts MUST be signed and protected from downgrade
  below security minimums.

## 15. Administrative and infrastructure security

- **SEC-INF-001:** Management endpoints, metrics, databases, event bus, secret
  manager, registries, and node APIs MUST not be exposed through public ingress.
- **SEC-INF-002:** NetworkPolicy/firewall rules MUST default deny between zones
  and permit documented service flows only.
- **SEC-INF-003:** Administrative access MUST use dedicated bastion/privileged
  access management, just-in-time grants, recording, and customer controls.
- **SEC-INF-004:** Infrastructure changes MUST use reviewed declarative code and
  produce deployment evidence; direct production mutation is emergency-only.
- **SEC-INF-005:** Kubernetes service accounts and cloud/on-prem identities MUST
  be unique per component and namespace/environment.
- **SEC-INF-006:** Backups, snapshots, and support exports are classified as the
  highest contained data and protected accordingly.
- **SEC-INF-007:** Non-production environments MUST not share production keys,
  credentials, databases, connectors, or identity clients.

## 16. Detection, response, and emergency controls

- **SEC-IR-001:** Security telemetry MUST detect repeated denies, bypass attempts,
  unusual approval patterns, credential misuse, cross-tenant probes, sandbox
  escape signals, policy drift, evidence gaps, and suspicious exports.
- **SEC-IR-002:** CISO controls MUST support tenant, agent, connection, skill,
  tool, model, template, policy, workload, and destination quarantine/revocation.
- **SEC-IR-003:** Emergency stop MUST block new actions immediately and safely
  terminate or isolate active execution without deleting evidence.
- **SEC-IR-004:** Incident evidence MUST include configuration/policy/release
  versions, affected identities/actions, timeline, containment, and recovery.
- **SEC-IR-005:** Response runbooks MUST cover identity compromise, connector
  compromise, prompt injection, sandbox escape, supply-chain compromise,
  evidence integrity failure, data exposure, and malicious administrator.
- **SEC-IR-006:** Customer notification and regulatory workflows MUST be
  configurable to contractual/jurisdictional obligations.

## 17. Compliance readiness

GA does not itself assert certification. It MUST provide control evidence that
can support customer programmes such as ISO 27001, SOC 2, NIST CSF/800-53,
CIS benchmarks, privacy obligations, and regulated record retention.

Required compliance artifacts:

- system description, trust/data-flow diagrams, and asset inventory;
- control-to-requirement and requirement-to-test traceability;
- access review, privileged access, and separation-of-duty reports;
- policy/release/change histories and approval evidence;
- vulnerability, penetration, dependency, SBOM, and remediation reports;
- backup/restore, DR, incident, tabletop, and continuity evidence;
- retention, legal hold, deletion, export, and data-flow inventories;
- supplier and licence inventory, cryptographic key ownership, and rotation;
- independent GA acceptance report and open-risk register.

## 18. Security acceptance gates

Security sign-off requires:

1. No open critical or high exploitable vulnerability without CISO-approved,
   time-bounded exception and compensating control.
2. Tenant isolation, privilege escalation, IDOR, SSRF, CSRF, injection, request
   smuggling, deserialization, secret leakage, and auth bypass testing complete.
3. Prompt-injection and action-bypass suite proves all supported effect routes
   reach AGS and the Action Gateway.
4. Planner compromise exercise proves no direct credentials, Docker/KVM/SPACE
   management, connector authority, or evidence-signing authority is available.
5. Sandbox/browser escape and egress testing passes on supported hardware/kernel.
6. Approval replay, digest substitution, stale policy, confused deputy,
   delegation amplification, and revoked identity tests pass.
7. Evidence tamper, omission, replica concurrency, anchor, export, restore, and
   verifier tests pass.
8. Signed offline bundle verification and malicious/downgrade artifact tests pass.
9. Independent penetration test findings are closed or formally accepted.
10. CISO signs the residual-risk register and production security runbooks.

