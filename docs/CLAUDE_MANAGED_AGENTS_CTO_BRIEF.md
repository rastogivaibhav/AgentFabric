# Claude Managed Agents CTO Brief

## Summary

We have completed the foundational platform work required for GOVAGN to support Claude Managed Agents as a governed runtime.

The key architectural shift is that managed-agent execution is now represented as first-class persisted state inside GOVAGN. We no longer rely on placeholder objects or only on span-derived runs to represent agent lifecycle.

## What Shipped

We added a managed-runtime schema and API layer covering:

- agent definitions
- environment definitions
- sessions
- session events
- tasks
- artifacts
- approval decisions

We also wired these records into:

- admin audit logging
- control-history evidence
- portal client hooks
- backward-compatible `runs` projection

## Why This Matters

This is the minimum credible architecture for enterprise Claude Managed Agents coverage.

Without first-class session and task objects, GOVAGN could observe model traffic but could not reliably answer operational questions such as:

- which session produced this result
- which task is blocked or awaiting approval
- what artifact came out of the task
- whether a task was approved, denied, or interrupted

Phase 2 solves that internal modeling problem.

## Strategic Position

This work supports the correct product stance:

- Anthropic hosts execution
- GOVAGN governs, audits, explains, and constrains execution

That is strategically preferable to building a competing hosted runtime now. It keeps GOVAGN aligned with its strengths in governance, budgets, policy, prompts, auditability, and enterprise controls.

## What We Did Not Build

We did not build:

- Anthropic upstream synchronization
- provider-side SSE ingestion
- a hosted executor
- sandbox infrastructure
- full tool-invocation lineage
- multiagent orchestration

That is intentional. The goal of this phase was to establish the internal control-plane model first.

## Business Impact

This phase improves GOVAGN in four ways.

First, it gives us a credible roadmap for Claude Managed Agents support that is compatible with enterprise governance requirements.

Second, it creates a reusable managed-runtime abstraction that can later support similar offerings from other vendors.

Third, it preserves existing reporting continuity by projecting managed tasks into `runs`.

Fourth, it gives us a clean base for execution-time policy and budget enforcement in the next phase.

## Technical Impact

The main system-of-record change is:

- `managed_sessions` and `managed_tasks` are now authoritative for managed runtime lifecycle
- `runs` remain an analytical projection

This is the right long-term direction. It avoids forcing hosted runtime semantics into a telemetry model that was designed around spans.

## Remaining Gaps

To reach customer-visible Claude Managed Agents coverage, we still need:

- Anthropic adapter and ingestion path
- managed runtime event streaming
- explicit tool invocation model
- approval queue UX
- runtime governance hooks before continuation or tool execution

## Recommended Next Investment

The next phase should focus on execution-time governance and upstream integration, not on building our own hosted executor.

Recommended order:

1. Anthropic managed-runtime adapter and event ingestion
2. policy and budget enforcement for session and task continuation
3. approval queue and operator UI
4. artifact and tool timeline UI

## Resourcing View

This looks like a contained control-plane expansion, not a new infrastructure business.

A practical next increment is:

- one backend engineer for provider adapter and governance hooks
- one frontend engineer for operational UI
- shared product and platform design for approval workflow semantics

## Recommendation

Continue down the control-plane path.

We now have the internal model needed to support Claude Managed Agents without diluting focus into hosted execution. The next investment should aim at live Anthropic integration and governance-in-the-loop behavior, which is where GOVAGN can differentiate commercially.
