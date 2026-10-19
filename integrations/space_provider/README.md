# AgentKraft × Perplexity SPACE — Computer broker G1

**Current status: contract CI plus a recorded local AGS → SPACE → two-app PUL
→ GovAgn proof. Connector effects remained dry-run. No live PUL connector,
real LLM, externally immutable evidence or independent GA acceptance.**

This adapter lives in AgentFabric because AgentFabric is the designated
GitHub Actions and evidence repository. AgentKraft remains responsible for
intent-based agent creation; the out-of-VM broker owns execution permissions.
Perplexity SPACE is the proposed Firecracker execution layer — **not**
Hugging Face Spaces.

Inspected SPACE revision: 09e922efd23b3c0d109ab64de7e567a901bcc61e
https://github.com/perplexityai/SPACE/blob/09e922efd23b3c0d109ab64de7e567a901bcc61e/examples/first_sandbox.py

## Implemented code

- space_client.py: native SPACE API for VM create, wait, command, SSE output,
  and delete. Network egress defaults to deny. HTTPS required off-loopback;
  no automatic redirects, inherited proxies or unsafe attachment URLs.
- ags_authorizer.py: fail-closed AGS Sovereign `/evaluate` adapter for launch
  and command authorization. It sends tenant/run identifiers and the exact
  action SHA-256, never raw intent, command arguments or infrastructure
  credentials. DENY, ESCALATE, approval-required, malformed and unavailable
  outcomes do not permit execution.
- pul_client.py: constrained PUL `/v1/execute` client for application
  operations. It preserves deny and approval-required outcomes with receipt
  identifiers, binds app identity and idempotency keys, and refuses redirects,
  inherited proxies, remote plaintext HTTP and malformed results.
- mission_broker.py: a trusted **outside-the-VM** broker with explicit
  authorisation callbacks for both VM allocation and every command.
  Missing/malformed/refused decisions and authoriser outages fail closed.
- SQLite journal: mission/tenant/run IDs, SHA-256 of intent, hashed
  authorisation-bound request plan, pending operation state, and receipt-like
  event metadata with UTC timestamps and a verifiable local hash chain.
  Arbitrary prompts, OAuth tokens, raw command arguments and command output
  are not written to the journal.
- SPACE idempotency keys are deterministically namespaced by tenant, run,
  mutation kind and caller request key, so separate tenants under one SPACE
  organisation cannot accidentally reuse the same idempotency key.
- Requests are journalled **before** network calls. Ambiguous create, command
  and delete outcomes enter needs_reconciliation. No second VM or repeated
  command is issued by the broker without reconciliation.
- Evidence is testable for same-host storage crashes/restarts; it is NOT yet
  anchored in GovAgn, a remote immutable store or a multi-node database.

## Safety limitations

The broker still accepts authorization callback interfaces, but the provided
`AGSPermitSource` is now the concrete AGS implementation. Unit tests use an
HTTP fixture and are not valid business approvals. Production must configure
a real tenant-scoped AGS operator identity and policies; never configure an
allow-all callback. PUL remains the separate application execution,
least-privilege, approval and receipt layer for GitHub, Gmail, Jira and other
connectors. The provided client submits operations but intentionally has no
administrative approval method. An AGS permit to run a sandbox command is not
a PUL permit to perform a business write.

A VM is not a permission boundary inside Salesforce, Gmail, Shopify etc.
A browser carrying write-capable credentials can bypass an API-only gateway;
use application-side least privilege and enforce the appropriate controls
regardless of whether the agent uses browser or API.

Local SQLite + self-hashed events provide crash recovery for one broker
instance, not immutable evidence or safe distributed orchestration. Root/admin
database writers can rewrite the hash chain. Persist external anchors and
use production-grade transactional outbox before enterprise claims.
Reconciliation after an ambiguous network operation still requires an
authorised human/operator workflow.

## CI (no actual VM)

    PYTHONPATH=integrations/space_provider python3 -m unittest discover \
      -s integrations/space_provider -p 'test_*.py' -v

The dedicated Actions workflow tests against a local HTTP fixture, records
source SHA-256s (including the journal and its tests), checks whether the
GitHub runner can access /dev/kvm, and stores logs with explicit gates.
Passing these tests **does not** imply any real Firecracker execution,
real AGS service decision, LLM/browser autonomy, PUL permission or GovAgn
delivery.

## Local live proof completed

On 2026-10-18 the adapter and broker were exercised against the local AGS
Sovereign 1.1.0 control plane and a local Linux/x86_64/KVM SPACE cluster at
revision `09e922efd23b3c0d109ab64de7e567a901bcc61e`. AGS issued distinct
ledger-backed PERMIT decisions for the launch and exact command fingerprint.
AgentFabric created a SPACE sandbox, ran a verified command in Firecracker
1.15.1, and deleted it. Both the AGS ledger and broker journal verified, and
API, pod and process-level cleanup found no orphan.

This is local development evidence, not production or GA acceptance. The WSL
worker image also omits two `socket transparent` nftables rules because the
stock WSL kernel lacks `CONFIG_NFT_SOCKET`; do not claim the complete network
enforcement path from this run.

The next proof must verify a denied network destination and refused PUL write.
It must also cover forced error, restart/reconciliation, tenant separation and
orphan cleanup.

## Local PUL policy proof completed

On 2026-10-18 a local PUL `v1-dev` runtime was started with isolated file
state, an explicit local admin boundary and no provider credentials. A
registered AgentFabric proof app explicitly denied
`repo.github.search_code`; PUL blocked it and issued a denial receipt. The app
then requested `repo.github.create_issue`; PUL queued an approval, executed
only after the separate operator approval call, and updated the receipt to
`approved`/`completed`. The GitHub connector reported `dry_run`, so no external
repository was changed.

This validates the local policy/approval/receipt contract. It is not a live
connector proof, a two-application workflow, or evidence that an admin token
is production-managed.

## Recorded two-application proof completed

On 2026-10-19 one local AgentFabric mission used real AGS permits and a real
SPACE Firecracker sandbox to produce a plan, then used two separately
registered PUL identities. The planner completed
`repo.github.search_code`; the executor's `repo.github.create_issue` remained
pending until a distinct operator approval. Both connector results were
deliberately `dry_run`, so no external repository changed.

The local GovAgn collector and PostgreSQL-backed gateway stored a seven-span
trace containing the AgentFabric run ID, both AGS decision IDs, sandbox ID,
both PUL receipt IDs, the PUL approval ID and cleanup status. The mission
journal and AGS ledger verified, and post-run API, pod and process checks found
no surviving sandbox. The recording includes the first attempt, which failed
closed during evidence validation and completed emergency cleanup, and the
successful retry.

This proves local cross-component correlation, not externally immutable
evidence, a live third-party connector, production secret management or GA.

## Gates

| Gate | Status |
|---|---|
| SPACE native HTTP request and failure contract | CI (fixture) |
| Same-host durable run journal and tenant separation | CI (local SQLite + fixture) |
| VM allocation and command gate required | CI (AGS HTTP fixture) |
| Real Firecracker VM | LOCAL DEV PASS (WSL2/KVM; not GA) |
| Real AGS permit for launch and command | LOCAL DEV PASS (not GA) |
| PUL HTTP client contract | CI (fixture) |
| PUL deny + approval + receipt | LOCAL DEV PASS (dry-run; not GA) |
| PUL two-application approval flow | LOCAL DEV PASS (dry-run; not GA) |
| PUL live connector | BLOCKED |
| GovAgn correlated trace delivery | LOCAL DEV PASS (local stack; not GA) |
| GovAgn externally immutable evidence | BLOCKED |
| Real LLM decisions and independent customer demonstration | BLOCKED |
| Security/resilience/operational acceptance | BLOCKED |
| AgentKraft Enterprise GA | NOT APPROVED |

**Next bounded integration:** verify denied network egress on native Linux,
then exercise forced failures and restart/reconciliation while retaining the
AGS, PUL, SPACE and GovAgn correlation chain. The broker itself must not run
inside the microVM.
