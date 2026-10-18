# AgentKraft / Perplexity SPACE computer provider (G0)

**Status: API contract implementation, NOT real Firecracker or GA proof.**

AgentFabric hosts the review branch, GitHub Actions and evidence. AgentKraft
remains the intent-to-agent product. This adapter targets the Perplexity SPACE
native API with Firecracker runtime, not Hugging Face Spaces.

Inspected SPACE revision: 09e922efd23b3c0d109ab64de7e567a901bcc61e
https://github.com/perplexityai/SPACE/blob/09e922efd23b3c0d109ab64de7e567a901bcc61e/examples/first_sandbox.py

## Features implemented in this slice

- Create a SPACE sandbox with runtime fc and deny-by-default egress.
- Poll both operation and VM readiness; accepted requests are not a running VM.
- Start a command session and verify the returned data-plane origin.
- Attach using a separate session token, consume SSE and require a zero exit code.
- Delete sandbox and await completed operation.
- No redirects or inherited proxies; HTTPS required outside loopback.
- Caller-provided stable idempotency keys; no automatic ambiguous retries.
- Metadata-only event callback; not yet persisted as GovAgn evidence.

SPACE management and connector credentials must stay outside the VM.
Consequential enterprise writes require independent AGS/PUL enforcement.
The provider does not deliver a graphical desktop or browser automation; both
require separate build/test work.

## Run contract tests (local HTTP fixture only)

    PYTHONPATH=integrations/space_provider python3 -m unittest discover -s integrations/space_provider -p 'test_*.py' -v

The CI workflow in .github/workflows/space-provider-contract.yml runs these
fixture tests on the dedicated review branch or PRs to agentfabric-dev.
Passing them does NOT establish that Firecracker ran.

## Real VM acceptance, manual and separately authorised

Requires an approved Linux x86_64/KVM or existing SPACE deployment, owned
credentials, appropriate TLS, isolated test systems and explicit acceptance
of potential infrastructure cost. Do not connect real customer credentials.
The GitHub workflow does not create VMs or invoke external LLMs.

    import os
    from space_client import SpaceComputerProvider
    provider = SpaceComputerProvider(
        os.environ["SPACE_API_URL"],
        os.environ["SPACE_DATA_PLANE_URL"],
        os.environ["SPACE_API_TOKEN"],
    )
    # Persist key + body before the request; reuse EXACT same key if uncertain.
    ref = provider.create(template="seed:latest", request_key="owned-mission-create-001")
    provider.wait_ready(ref)
    try:
        result = provider.command(ref.sandbox_id, argv=["sh", "-c", "echo hello"],
                                  request_key="owned-mission-command-001")
        assert result.stdout == b"hello\n"
    finally:
        provider.delete(ref.sandbox_id, request_key="owned-mission-delete-001")

This example creates a real VM if executed on a configured SPACE deployment;
it is documentation only and has not been run in this iteration.

## Verification and release gates

- CI fixture contract: execute, record GitHub SHA / Action URL.
- Real Firecracker VM create, command, cleanup: BLOCKED until provisioned
  and independently inspected test infrastructure is available.
- Intent compiler and two scoped software connections: BLOCKED for this slice.
- Live LLM, browser driver, AGS/PUL, GovAgn correlated receipts: BLOCKED.
- Cross-tenant isolation, restart recovery, egress escapes: BLOCKED.
- Independent customer, security and operational gates: BLOCKED.
- AgentKraft Enterprise GA: NOT APPROVED.

NEXT: a durable computer broker that binds mission/tenant/run IDs to SPACE
sandbox leases, keeps an operation journal, enforces AGS/PUL on writes, and
streams real VM events to GovAgn independently of the guest.
