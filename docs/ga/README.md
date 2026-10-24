# AgentFabric Enterprise GA Specification Set

**Version:** 1.0-draft  
**Baseline date:** 24 October 2026  
**Target:** Enterprise-controlled and air-gapped General Availability  
**Normative status:** Draft for architecture, product, CIO, CISO, and engineering approval

This specification set converts the
[architecture assessment](../architecture-review-and-target-design-2026-10-24.md)
into buildable and testable requirements for Enterprise GA. It does not claim
that the requirements are implemented today.

## GA definition

AgentFabric Enterprise is GA only when an organisation can install, operate,
upgrade, recover, and audit the platform inside its trust boundary and when an
entitled employee can create and use a persistent AI teammate without the
model, employee, skill, plugin, or client obtaining uncontrolled authority.

The GA product must support this complete outcome:

```text
employee identity
  -> approved teammate and entitlements
  -> durable task and planner
  -> typed action proposal
  -> AGS decision and optional human approval
  -> PUL and/or SPACE execution
  -> immutable evidence, cost settlement, and employee-visible result
```

An isolated computer by itself is not GA. A policy decision without an
unavoidable enforcement path is not GA. A dashboard without lossless action
evidence is not GA. A local demo that requires public package downloads is not
air-gap GA.

## Specification documents

| Document | Purpose | Primary reviewers |
|---|---|---|
| [Product requirements](product-requirements.md) | Personas, capabilities, journeys, functional requirements, product acceptance | Product, enterprise architecture, employee experience |
| [System architecture and contracts](system-architecture.md) | Services, boundaries, state machines, APIs, events, data model, failure semantics | Solution and application architecture |
| [Security, governance, and compliance](security-governance.md) | Trust model, identity, policy, approval, isolation, evidence, privacy, supply chain | CISO, security architecture, risk, compliance |
| [Platform, operations, and air-gap release](platform-operations.md) | Deployment, HA, SLOs, capacity, backup, DR, packaging, install, upgrade, support | Infrastructure architecture, SRE, platform engineering |
| [Verification and GA acceptance](verification-acceptance.md) | Test strategy, evals, performance, failure injection, acceptance gates, evidence pack | QA, security test, SRE, independent acceptance |
| [Delivery plan and backlog](delivery-plan.md) | Workstreams, dependencies, milestones, ownership, Definition of Done | Programme, engineering, product, architecture |

## Requirement language

- **MUST / SHALL:** mandatory for the stated GA profile.
- **MUST NOT / SHALL NOT:** prohibited for the stated GA profile.
- **SHOULD:** expected unless an approved ADR records a justified exception.
- **MAY:** optional and must not weaken mandatory controls.

Requirement identifiers are stable. A removed requirement remains reserved and
is marked superseded; identifiers are never reused.

| Prefix | Area |
|---|---|
| `PRD` | Product and user capability |
| `ARC` | Architecture and service boundary |
| `CON` | Contracts, APIs, and events |
| `DAT` | Data and consistency |
| `SEC` | Security and governance |
| `EVD` | Evidence and audit |
| `OPS` | Operations and SRE |
| `AIR` | Air-gap and sovereign deployment |
| `REL` | Release and upgrade |
| `TST` | Verification and acceptance |

## Non-negotiable invariants

1. The planner proposes; it never exercises enterprise authority directly.
2. Every external side effect and sensitive read passes through the Governed
   Action Gateway.
3. AGS is the only pre-execution policy decision point.
4. Approval is digest-bound, single-use, expiring, and revalidated before use.
5. PUL is the only broker of enterprise application credentials and delegated
   authority.
6. SPACE is the production computer/browser/software execution boundary.
7. GovAgn observes and explains; it does not authorize what it measures.
8. Every acknowledged effect has a complete, lossless evidence record.
9. Tenant identity comes from authenticated identity, never an untrusted header
   or model argument.
10. No production component downloads unapproved code, images, models, skills,
    or packages from the public internet.
11. No employee-controlled YOLO or governance bypass exists.
12. Unknown or ambiguous execution outcomes are reconciled and never blindly
    repeated.

## GA deployment profiles

The same product contracts apply to three profiles:

| Profile | Description | GA status |
|---|---|---|
| Connected enterprise | Customer-controlled platform with approved outbound connector/model zones | Required |
| Sovereign/on-premises | Customer datacentre or sovereign cloud; internal identity, PKI, stores, and registries | Required |
| Fully air-gapped | No runtime internet path; all artifacts, models, licences, updates, and diagnostics supplied offline | Required for the Enterprise Air-Gap edition |

The local Windows/WSL2 launcher remains a development/evaluation profile and is
not a production deployment profile.

## Decision authority

| Decision | Accountable authority |
|---|---|
| Product scope and employee experience | Product executive / CIO delegate |
| Security invariants and risk acceptance | CISO delegate |
| Architecture boundaries and contracts | Architecture Review Board |
| Data classification and retention | Data owner and privacy/compliance |
| GA release readiness | Product, CISO, SRE, and independent acceptance sign-off |
| Emergency production override | Dual-authorized CIO/CISO break-glass process |

No single engineering team can waive a GA invariant. Exceptions require an ADR,
risk acceptance, bounded duration, compensating control, owner, and expiry.

## Traceability contract

Every implementation change for GA must reference:

- one or more requirement IDs;
- an approved architecture decision when a boundary changes;
- automated tests or a documented manual acceptance procedure;
- observability and evidence emitted by the feature;
- operational ownership and rollback behaviour;
- threat-model impact when authority, data, network, or execution changes.

The release evidence pack must map every MUST requirement to one of:

- automated passing test;
- independently witnessed procedure and signed result;
- approved non-applicability statement;
- approved, time-bounded release exception.

Open or failed mandatory requirements block GA.

