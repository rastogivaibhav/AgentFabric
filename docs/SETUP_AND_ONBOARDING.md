# Setup and Onboarding

## Purpose

This guide helps platform teams, administrators, and application teams get Govagn running and onboard their first workloads.

It covers:

- environment setup
- admin bootstrap
- provider and policy setup
- workload onboarding
- trace and governance validation

## Who Should Use This Guide

- platform engineering
- enterprise architecture
- AI platform teams
- security and governance teams
- application teams onboarding AI workloads

## Choose Your Starting Path

Use this guide based on your goal:

- local evaluation: [QUICKSTART.md](QUICKSTART.md)
- single-team or single-program deployment: [INSTALL_SINGLE_TENANT.md](INSTALL_SINGLE_TENANT.md)
- shared platform deployment: [INSTALL_MULTI_TENANT.md](INSTALL_MULTI_TENANT.md)

## Deployment Paths

### Local evaluation
Use this when you want to:

- evaluate the platform quickly
- test instrumentation
- validate traces, policies, and prompts locally

Recommended path:

- Docker Compose

### Production-like evaluation
Use this when you want to:

- validate operational behavior
- exercise readiness scripts
- test multi-user or multi-tenant flows

Recommended path:

- Helm or production Compose

## Prerequisites

Before starting, make sure you have:

- Docker and Docker Compose, or Kubernetes and Helm
- PostgreSQL and Redis connectivity
- environment secrets ready
- a browser for portal access
- at least one provider credential for proxied runtime traffic

## Step 1: Deploy the Platform

### Option A: Local Docker Compose
Bring up the stack, apply migrations, and verify service health.

Expected outcomes:

- portal is reachable
- API gateway is reachable
- collector is reachable
- database migrations are applied
- admin login is available

### Option B: Helm / Kubernetes
Deploy the runtime stack, apply configuration, expose ingress, and validate readiness.

Expected outcomes:

- gateway readiness checks pass
- collector readiness checks pass
- database and Redis connectivity is healthy
- required secrets are present

## Step 2: Configure Core Environment Settings

At minimum, configure:

- authentication mode
- admin credentials
- database connection
- Redis connection
- encryption or vault key material
- portal base URL and API base URL
- environment label such as `dev`, `staging`, or `prod`

For production-like environments, also configure:

- TLS
- ingress or reverse proxy
- CORS allowlist
- password policy or SSO path
- backup destination and retention approach

## Step 3: Sign In as Administrator

Use the initial admin account to access the portal.

As an administrator, validate the following first:

- login works
- system health pages load
- traces page loads
- policies page loads
- cost page loads
- prompts page loads
- evals page loads

## Step 4: Register Provider Access

Add provider credentials or managed access through the API gateway.

Typical onboarding flow:

1. register a provider key
2. choose provider scope
3. validate provider metadata
4. send a test proxied request
5. verify trace and cost capture

You should confirm:

- requests succeed through the controlled path
- provider, model, and usage details are captured
- cost attribution appears in trace detail

## Step 5: Configure Governance Baseline

Before onboarding users broadly, create a minimum governance baseline:

- one budget rule
- one pricing rule
- one policy or guardrail
- one audit validation path
- one prompt lifecycle object for testing

Suggested first controls:

- deny or warn on clearly disallowed content
- configure a low test budget
- create one prompt version and release tag
- validate trace-to-prompt linkage

## Step 6: Onboard the First Workload

There are two primary onboarding patterns.

### Pattern A: Instrumented workloads
Use the SDK or telemetry path to emit runtime traces.

Expected metadata to capture:

- tenant or team identity
- service name
- prompt metadata where available
- runtime model and provider context

### Pattern B: Proxied workloads
Send model traffic through the gateway.

Expected outcomes:

- policy is enforced
- pricing is calculated
- usage is attributed
- audit and trace detail are visible

## Step 7: Validate Trace, Cost, and Policy Behavior

Once your first workload is live, validate:

- traces appear in the portal
- span detail is readable and enriched
- cost breakdown is visible
- policy decisions are explainable
- audit records are present
- prompt linkage appears where configured

Recommended checks:

- compare two traces
- preview a policy decision
- simulate a guardrail
- inspect a cost rule match
- review prompt release linkage

## Step 8: Run Environment Proof Scripts

For staging or production-like setups, run the operational validation scripts.

Use them to validate:

- stack health
- proxy path
- release-candidate validation
- governance scenarios
- GA gate readiness

This turns setup into evidence, not assumption.

## Step 9: Onboard Additional Teams

For broader rollout, define a repeatable team onboarding pattern.

Recommended onboarding packet:

- tenant or environment assignment
- provider access policy
- prompt lifecycle convention
- trace naming convention
- budget threshold
- ownership contacts
- release approval flow

## Onboarding Checklist

### Platform team checklist

- environment deployed
- migrations applied
- secrets configured
- readiness checks passing
- provider route verified
- backup approach documented

### Governance checklist

- pricing rule exists
- budget rule exists
- at least one active policy exists
- audit trail verified
- policy simulation tested

### Application team checklist

- workload instrumented or proxied
- service metadata visible
- traces flowing
- prompt metadata visible where applicable
- cost attribution visible
- expected guardrails validated

## Recommended First Pilot

For a first real pilot, start with:

- one team
- one tenant
- one provider path
- one governed prompt lifecycle
- one budget policy
- one reporting cadence

Success criteria:

- trace visibility works end to end
- governance behavior is explainable
- cost can be attributed clearly
- onboarding takes hours or days, not weeks

## Common Setup Mistakes

- deploying services without applying migrations
- testing proxy flows before registering provider keys
- enabling policy without validating baseline rules
- treating prompt lifecycle as optional metadata and not wiring it early
- running a production-like environment without readiness proof scripts
- broad rollout before one tenant is proven end to end

## What Good Looks Like

A healthy first deployment has:

- one working proxied or instrumented workload
- visible traces and cost breakdown
- at least one explainable policy decision
- one prompt release linked to runtime traces
- one successful staging or pilot validation run

## Next Documents

After this guide, use:

- [API_GUIDE.md](API_GUIDE.md)
- [ARCHITECTURE.md](ARCHITECTURE.md)
- [INSTALL_SINGLE_TENANT.md](INSTALL_SINGLE_TENANT.md)
- [INSTALL_MULTI_TENANT.md](INSTALL_MULTI_TENANT.md)
- [PRODUCTION_CHECKLIST.md](PRODUCTION_CHECKLIST.md)
