# Claude Managed Agents Adaptation Plan

## Purpose

This document turns the April 2026 Claude Managed Agents launch into a repo-specific adaptation plan for GOVAGN.

The goal is not to clone Anthropic's hosted runtime. The goal is to let GOVAGN govern, observe, and explain workloads that execute on Claude-managed infrastructure while preserving GOVAGN's control-plane positioning.

## Source Notes

As of April 9, 2026, the exact overview URL we were given was not directly fetchable from this environment. This plan is based on the current official Claude docs around:

- sessions: `https://code.claude.com/docs/en/agent-sdk/sessions`
- permissions: `https://code.claude.com/docs/en/agent-sdk/permissions`
- agent loop: `https://platform.claude.com/docs/en/agent-sdk/agent-loop`
- tool use: `https://platform.claude.com/docs/en/agents-and-tools/tool-use/how-tool-use-works`
- feature overview: `https://platform.claude.com/docs/en/build-with-claude/overview`

Inference from those docs plus the public-beta launch summary on April 8, 2026:

- Claude Managed Agents packages a hosted execution runtime around Claude's existing agent primitives
- the important primitives are session continuity, tool execution, approvals/permissions, hooks/guardrails, artifacts, and long-running task state
- Anthropic is moving "agent infrastructure" from customer code into their managed platform

That matters because GOVAGN currently governs traffic and telemetry well, but it does not yet model hosted agent execution as a first-class runtime.

## Strategic Position

Recommended product position:

- Do not build an Anthropic competitor runtime first
- Do build a control-plane adapter for external managed runtimes, starting with Claude Managed Agents
- Treat Claude Managed Agents as a governed execution substrate that GOVAGN can observe, constrain, cost, and audit

Why:

- GOVAGN is already strongest in governance, observability, prompts, budgets, rollout control, and audit
- reproducing managed execution would require sandboxing, worker orchestration, checkpointing, artifact persistence, approval inboxes, retries, and hosted lifecycle guarantees
- that is a bigger product shift than what is needed to "cover" Managed Agents in a credible enterprise way

## Current Repo Map

### Already aligned

- Anthropic SDK patching exists in `agent-sdk/govagn/__init__.py`
  - `instrument()` initializes provider-level tracing
  - `_patch_anthropic()` wraps `Anthropic().messages.create`
- Anthropic proxy routing exists in `api-gateway/internal/proxy/provider_registry.go`
- Anthropic request/usage parsing exists in `api-gateway/internal/proxy/parsers_anthropic.go`
- runs/agents/traces/live stream APIs already exist in `docs/openapi.yaml`
- control history and evidence bundles already exist in `api-gateway/internal/memory/service.go`
- portal surfaces scorecards, runs, prompts, rollouts, policies, memory, and traces

### Current limitation

GOVAGN mostly derives agent state from spans after execution:

- `api-gateway/internal/store/postgres.go` projects runs from spans via `upsertRunsFromSpans()`
- `runs` and `agents` are analytics and observability objects, not authoritative execution lifecycle objects

This works for self-managed SDKs and proxy traffic, but it is too thin for Claude Managed Agents where important semantics live above a single model request:

- session creation and continuation
- task lifecycle
- approval requests
- server-executed tools
- artifacts and outputs
- subagent delegation
- forks / resume / branch semantics

## Gap Analysis

### 1. SDK gap

Current state:

- Anthropic support is message-level only
- tool loops, approvals, and session state are not modeled explicitly

Required:

- capture managed-agent lifecycle events, not just `messages.create`
- preserve identifiers for session, task, approval, tool, artifact, and subagent lineage

### 2. Gateway gap

Current state:

- gateway APIs are read-heavy for agents and runs
- no first-class execution resources for hosted runtime objects

Required:

- first-class resources for agent definitions, sessions, tasks, approvals, and artifacts
- an adapter layer that normalizes Claude Managed Agents data into GOVAGN's control-plane model

### 3. Data model gap

Current state:

- `runs` are projected from spans
- "memory" means governance memory and evidence, not conversation/task memory

Required:

- canonical execution tables for hosted runtimes
- explicit differentiation between:
  - execution memory
  - governance memory
  - prompt/release lineage

### 4. Portal gap

Current state:

- portal is optimized for observability and governance after the fact

Required:

- operational views for active managed agents:
  - active tasks
  - approval inbox
  - tool timeline
  - artifacts
  - session resume/fork state

## Architecture Direction

Add a new runtime class:

- `runtime_type = self_managed_sdk | proxied_llm | managed_agent`

Add a Claude-specific adapter:

- `runtime_provider = anthropic`
- `runtime_product = claude_managed_agents`

Normalization rule:

- spans remain the cross-runtime observability fabric
- sessions/tasks/approvals/artifacts become first-class entities
- runs remain a derived summary view, not the source of truth for managed agents

## Phase Plan

## Phase 1: Claude Managed Runtime Adapter

Objective:

- let GOVAGN ingest and explain Claude Managed Agents activity without hosting execution

Deliverables:

- new adapter package in the gateway for managed runtime normalization
- canonical metadata mapping for Claude-managed sessions and tasks
- live event ingestion path for managed runtime events
- portal indicators showing when a run came from a managed runtime

Suggested file additions:

- `api-gateway/internal/managedruntime/service.go`
- `api-gateway/internal/managedruntime/anthropic.go`
- `api-gateway/internal/models/managed_runtime.go`
- `api-gateway/internal/store/managed_runtime.go`

Suggested file changes:

- `agent-sdk/govagn/__init__.py`
- `api-gateway/internal/models/models.go`
- `api-gateway/internal/handlers/handlers.go`
- `docs/openapi.yaml`
- `portal/src/hooks/api.ts`

New normalized fields:

- `runtime_type`
- `runtime_provider`
- `runtime_product`
- `managed_session_id`
- `managed_task_id`
- `managed_agent_id`
- `approval_id`
- `artifact_id`
- `checkpoint_id`
- `subagent_id`
- `parent_task_id`
- `server_tool_count`
- `client_tool_count`

Recommended behavior:

- if only coarse telemetry is available, still create a managed session/task envelope
- attach raw provider payloads for explainability and later parser upgrades
- keep all provider-specific details in metadata, but normalize the operator-critical fields

## Phase 2: First-Class Execution Objects

Objective:

- stop overloading projected runs as the only agent lifecycle object

Add tables:

- `agent_definitions`
- `agent_sessions`
- `agent_tasks`
- `tool_invocations`
- `approval_requests`
- `artifacts`

Suggested semantics:

- `agent_definitions`: what is deployed or registered
- `agent_sessions`: long-lived conversational or task context
- `agent_tasks`: one unit of work within a session
- `tool_invocations`: every tool request/result pair, including server tools where visible
- `approval_requests`: explicit human approval checkpoints
- `artifacts`: files, reports, outputs, bundles, deliverables

Important rule:

- `runs` should remain queryable for continuity, but become a summarized analytical projection over `agent_tasks` plus spans

## Phase 3: Governance at Execution Time

Objective:

- apply GOVAGN controls before or during managed-agent actions, not only after ingest

Leverage existing modules:

- policy engine in `api-gateway/internal/policy`
- budgets in `api-gateway/internal/budget`
- prompt releases in `api-gateway/internal/prompts`
- rollout logic in `api-gateway/internal/rollouts`
- evidence/control history in `api-gateway/internal/memory`

New enforcement points:

- pre-task validation
- pre-tool policy check
- approval routing
- budget-aware pause/deny
- prompt release pinning
- rollout-aware model assignment

This is the key enterprise differentiator:

- Anthropic runs the agent
- GOVAGN governs whether the agent is allowed to continue, use a tool, exceed budget, switch prompt release, or emit an artifact into a sensitive workflow

## Phase 4: Portal Operations Surface

Objective:

- make managed-agent operations visible in the same way traces are visible today

Suggested new pages:

- `ManagedAgentsPage`
- `ManagedAgentDetailPage`
- `SessionDetailPage`
- `TaskDetailPage`
- `ApprovalQueuePage`
- `ArtifactsPage`

Suggested upgrades to existing pages:

- `AgentsPage`: add runtime/source badges and managed-runtime health
- `RunDetailPage`: add session/task/approval/artifact panels
- `LiveStream`: show managed task state transitions and approval waits
- `MemoryPage`: split governance evidence from execution memory

## Phase 5: Cross-Runtime Abstraction

Objective:

- make Claude Managed Agents the first adapter, not the only one

Future-compatible interfaces:

- `ManagedRuntimeAdapter`
- `ExecutionEnvelope`
- `ExecutionEvent`
- `ApprovalEvent`
- `ArtifactRecord`

Why:

- if OpenAI, Google, or other vendors offer similar hosted agent runtimes, GOVAGN should ingest them through the same control-plane model

## Concrete Repo Touch Points

### Python SDK

`agent-sdk/govagn/__init__.py`

Current role:

- provider-level instrumentation

Changes:

- add explicit support for Anthropic tool loop metadata
- capture session IDs when available
- distinguish plain Anthropic message calls from managed-agent executions
- emit attributes for approvals, artifacts, and task lifecycle when surfaced by SDK responses

### Gateway models

`api-gateway/internal/models/models.go`

Changes:

- extend `Run`
- extend `Agent`
- add managed-runtime structs instead of burying everything in `Metadata`

Prefer:

- strongly typed structs for canonical managed-runtime fields

Avoid:

- pushing everything into untyped JSON and losing operator-level queryability

### Gateway store

`api-gateway/internal/store/postgres.go`

Current issue:

- `upsertRunsFromSpans()` assumes spans are the source of truth

Changes:

- keep span projection for backward compatibility
- add authoritative write paths for managed sessions, tasks, tools, and approvals
- generate run projections from those entities plus spans, not only from spans

### Gateway handlers and API

`api-gateway/internal/handlers/handlers.go`
`docs/openapi.yaml`

Add endpoints:

- `GET /api/v1/managed-agents`
- `GET /api/v1/managed-agents/{agentId}`
- `GET /api/v1/sessions/{sessionId}`
- `GET /api/v1/sessions/{sessionId}/tasks`
- `GET /api/v1/tasks/{taskId}`
- `POST /api/v1/tasks/{taskId}/approve`
- `POST /api/v1/tasks/{taskId}/deny`
- `GET /api/v1/tasks/{taskId}/artifacts`

Optional later:

- `POST /api/v1/managed-agents/register`
- `POST /api/v1/managed-runtime/ingest`

### Governance memory

`api-gateway/internal/memory/service.go`

Changes:

- keep current governance evidence behavior
- add explicit category coverage for managed-agent approvals, pauses, denials, and artifact releases

Do not rename this subsystem until execution memory is introduced separately.

Instead:

- keep "memory" here as control-plane memory
- introduce `execution_memory` as a distinct model later if needed

### Portal API hooks

`portal/src/hooks/api.ts`

Changes:

- add typed models for managed agents, sessions, tasks, approvals, and artifacts
- preserve current runs/agents hooks for compatibility
- expose runtime badges and lifecycle queries

## Minimum Viable Coverage

If we want the smallest credible implementation, Phase 1 should be considered done when:

- Anthropic managed-agent activity can be identified separately from plain Anthropic messages
- GOVAGN can show managed session ID, task ID, status, model, tool counts, total cost, and policy/budget decisions
- traces and live stream views can link back to managed session/task objects
- evidence bundles can include managed-agent approvals and artifacts

That would already let GOVAGN say:

- "we govern and observe Claude Managed Agents"

without claiming:

- "we host a replacement for Claude Managed Agents"

## What Not To Build First

Do not start by building:

- a full sandboxed executor
- local bash/file tool hosting for Anthropic-style agents
- checkpointing/replay infrastructure
- multi-tenant hosted worker orchestration

Those are expensive and pull GOVAGN away from its strongest position.

## Recommended Build Order

1. Add managed-runtime types and storage
2. Add Anthropic managed-runtime adapter and ingestion path
3. Extend API and portal for session/task/approval visibility
4. Route existing policy, budget, prompt, and rollout logic into managed-runtime decisions
5. Only then evaluate whether any hosted execution capability is strategically necessary

## Success Criteria

GOVAGN should be able to answer all of the following for a Claude Managed Agent run:

- Which managed agent, session, and task produced this output?
- Which model, prompt release, and environment were used?
- Which tools ran, and where did they execute?
- Which approvals were requested, granted, denied, or timed out?
- What did it cost, and which budget or pricing rules applied?
- Which policy or guardrail decisions changed execution?
- Which artifact or downstream output came out of the task?
- What evidence bundle can be exported for audit or incident review?

If GOVAGN can answer those questions reliably, it will cover Claude Managed Agents in a way that is useful to enterprise platform, security, and governance teams.
