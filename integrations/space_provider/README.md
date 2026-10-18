# AgentKraft × Perplexity SPACE — Computer broker G1

**Current status: local API-contract CI only. No real Firecracker VM, real LLM,
real application permission, AGS/PUL integration or independent GA acceptance.**

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

The launch and command authorisation callbacks are *interfaces*, not actual
AGS/PUL service calls. The unit tests pass explicit fixture permits; they
are not valid business approvals. Do not configure a production allow-all
callback. Real authorisation must be independently verifiable against
tenant, run, exact action/policy fingerprint, expiry and a real AGS/PUL
decision/permit. Enforce business writes outside the agent's control.

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
real LLM, browser autonomy, PUL permission or GovAgn delivery.

## Live proof needed to unlock the next gate

Requires a separately authorised, already-available Linux x86_64/KVM host
or SPACE cluster with known revision, trusted TLS, approval for infrastructure
costs, safe test software and non-customer credentials. The current connected
DigitalOcean account had no droplets; the inspected GitHub runner lacked usable
KVM. **No VM was launched in this iteration.**

On a genuinely available host, route a reviewed AgentKraft mission through
this broker, with a real AGS/PUL permit source that validates the exact
fingerprint and a durable external evidence sink. Verify:
1. Real SPACE operation IDs and Firecracker VM identity, not fixture IDs.
2. Real process output plus exit code, using session-scoped attachment.
3. A denied network destination and a refused write, verified outside the VM.
4. Two scoped test applications and an actual LLM/browser execution.
5. Forced error, restart/reconciliation, tenant separation and VM cleanup.
6. GovAgn receipts correlated to tenant/run/sandbox and independently
   checked against the underlying actions.

## Gates

| Gate | Status |
|---|---|
| SPACE native HTTP request and failure contract | CI (fixture) |
| Same-host durable run journal and tenant separation | CI (local SQLite + fixture) |
| VM allocation and command gate required | CI (fixture permit only) |
| Real Firecracker VM and isolated browser | BLOCKED (no approved KVM host) |
| Real AGS/PUL permit and safe two-application access | BLOCKED |
| GovAgn external durable evidence | BLOCKED |
| Real LLM decisions and independent customer demonstration | BLOCKED |
| Security/resilience/operational acceptance | BLOCKED |
| AgentKraft Enterprise GA | NOT APPROVED |

**Next bounded integration:** implement/test actual AGS/PUL decision exchange
and GovAgn external evidence sink, then run on an approved real SPACE host.
The broker itself must not run inside the microVM.
