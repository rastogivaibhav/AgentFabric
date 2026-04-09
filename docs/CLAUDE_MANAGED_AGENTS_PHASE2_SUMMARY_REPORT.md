# Claude Managed Agents Phase 2 Summary Report

## Executive Summary

Phase 2 is complete.

GOVAGN now models Claude Managed Agents execution as first-class persisted control-plane objects instead of relying on placeholder responses or span-derived run projections alone. The system now stores and serves managed agents, environments, sessions, session events, tasks, artifacts, and approval decisions through the API gateway and portal client contracts.

This phase establishes the internal system of record required for Claude Managed Agents coverage. It does not yet implement live upstream synchronization with Anthropic Managed Agents endpoints.

## Objectives

Phase 2 was intended to:

- replace Phase 1 scaffold responses with real persistence
- make session and task objects authoritative
- preserve compatibility with existing `runs` analytics by projecting managed tasks into runs
- expose admin-safe write APIs for managed runtime operations
- keep governance audit and control-history coverage for managed runtime actions

## Delivered

### 1. Database schema for managed runtime objects

Added migration:

- [0020_managed_runtime.up.sql](C:/Users/vrast/Documents/Agentic%20Code/files/deploy/migrations/0020_managed_runtime.up.sql)
- [0020_managed_runtime.down.sql](C:/Users/vrast/Documents/Agentic%20Code/files/deploy/migrations/0020_managed_runtime.down.sql)

New tables:

- `managed_agents`
- `managed_environments`
- `managed_sessions`
- `managed_session_events`
- `managed_tasks`
- `managed_artifacts`
- `managed_task_decisions`

### 2. Strongly typed managed runtime model layer

Added:

- [managed_runtime.go](C:/Users/vrast/Documents/Agentic%20Code/files/api-gateway/internal/models/managed_runtime.go)

This file now defines:

- canonical runtime objects
- create/upsert request payloads
- approval decision payloads

### 3. Persistent store implementation

Added:

- [managed_runtime.go](C:/Users/vrast/Documents/Agentic%20Code/files/api-gateway/internal/store/managed_runtime.go#L1)

Key capabilities:

- upsert and fetch managed agents and environments
- create and query managed sessions
- create and query session events
- upsert and query tasks
- create and query artifacts
- approve and deny tasks with decision recording
- project managed tasks into `runs` for backward-compatible analytics

Important implementation points:

- session events increment and persist session counters
- task decisions emit durable decision records
- task approval and denial create session events
- managed task state syncs into the existing `runs` table via [syncManagedTaskRun](C:/Users/vrast/Documents/Agentic%20Code/files/api-gateway/internal/store/managed_runtime.go#L921)

### 4. Real service layer

Added:

- [service.go](C:/Users/vrast/Documents/Agentic%20Code/files/api-gateway/internal/managedruntime/service.go#L1)

This replaces the prior no-op implementation and provides:

- store-backed read and write operations
- `pgx.ErrNoRows` normalization to `managedruntime.ErrNotFound`
- request-to-model translation for all Phase 2 commands

### 5. Gateway handlers and routes

Updated:

- [handlers.go](C:/Users/vrast/Documents/Agentic%20Code/files/api-gateway/internal/handlers/handlers.go#L956)
- [main.go](C:/Users/vrast/Documents/Agentic%20Code/files/api-gateway/cmd/server/main.go#L285)

New managed runtime endpoints now include:

- `GET /api/v1/managed-agents`
- `PUT /api/v1/managed-agents`
- `GET /api/v1/managed-agents/{agentId}`
- `GET /api/v1/managed-environments`
- `PUT /api/v1/managed-environments`
- `GET /api/v1/managed-environments/{environmentId}`
- `GET /api/v1/managed-sessions`
- `POST /api/v1/managed-sessions`
- `GET /api/v1/managed-sessions/{sessionId}`
- `GET /api/v1/managed-sessions/{sessionId}/events`
- `POST /api/v1/managed-sessions/{sessionId}/events`
- `GET /api/v1/managed-sessions/{sessionId}/tasks`
- `PUT /api/v1/managed-tasks`
- `GET /api/v1/managed-tasks/{taskId}`
- `GET /api/v1/managed-tasks/{taskId}/artifacts`
- `POST /api/v1/managed-tasks/{taskId}/artifacts`
- `POST /api/v1/managed-tasks/{taskId}/approve`
- `POST /api/v1/managed-tasks/{taskId}/deny`

Write endpoints are protected with existing admin middleware.

### 6. Audit and control-history integration

Managed runtime writes now flow into the existing governance surfaces through:

- admin audit logging
- control history records

This applies to:

- agent and environment creation and update
- session creation
- event append actions
- task creation and update
- artifact creation
- task approval and denial

### 7. OpenAPI and portal client contract updates

Updated:

- [openapi.yaml](C:/Users/vrast/Documents/Agentic%20Code/files/docs/openapi.yaml)
- [api.ts](C:/Users/vrast/Documents/Agentic%20Code/files/portal/src/hooks/api.ts)

Portal coverage now includes:

- typed managed runtime objects
- read hooks for agents, environments, sessions, events, tasks, artifacts
- mutation hooks for create and update operations
- approval and denial task actions

## What Changed Architecturally

Before Phase 2:

- managed runtime support was contract-first but not real
- managed sessions and tasks were not persisted
- approval and denial endpoints returned placeholder `404` behavior

After Phase 2:

- managed runtime execution objects are durable records
- sessions and tasks are authoritative control-plane entities
- `runs` are still available, but managed tasks now feed them as a compatibility projection

This is the first point where GOVAGN can credibly claim internal support for managed-runtime execution lifecycles rather than just telemetry placeholders.

## Verification

Completed successfully:

- `go test ./...` in `api-gateway/`
- `npm run build` in `portal/`

## Known Gaps

Phase 2 intentionally does not yet include:

- live synchronization from Anthropic Managed Agents APIs
- SSE ingestion from upstream managed session events
- native `tool_invocations` table or full tool execution timeline
- dedicated approval queue UI pages
- provider adapter logic for the `managed-agents-2026-04-01` beta transport

## Risks and Constraints

### Product risk

The system now has an internal managed-runtime model, but until upstream ingestion is added, records must be created by GOVAGN-side workflows or internal integrations.

### Data-shape risk

The current schema is strong enough for sessions, tasks, artifacts, and approvals, but Anthropic preview features like outcomes, multiagent lineage, and execution memory may require additional tables or columns in later phases.

### Operational risk

`runs` remain a compatibility projection. Teams should treat managed sessions and managed tasks as the source of truth for execution lifecycle going forward.

## Recommended Next Steps

### Phase 3 priority

Implement execution-time governance:

- policy checks before tool or task continuation
- budget-based pause and deny decisions
- prompt release pinning for managed runtime sessions
- explicit approval routing behavior

### Phase 4 priority

Add portal operations views:

- managed agent detail
- session timeline
- task detail
- approval inbox
- artifact browser

### Adapter priority

Add the Anthropic managed-runtime adapter:

- upstream session and event ingestion
- provider payload normalization
- SSE consumption and persistence
- beta-header-aware integration path

## Bottom Line

Phase 2 delivered the core internal platform change that GOVAGN needed: managed runtime execution is now represented as durable control-plane state. This turns Claude Managed Agents support from a read-only concept into a real product surface that can be governed, audited, extended, and integrated.
