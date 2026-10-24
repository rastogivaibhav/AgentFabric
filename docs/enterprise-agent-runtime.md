# Enterprise Agent Runtime

AgentFabric now owns the complete agent product surface rather than only
observing agents built elsewhere.

```text
User / enterprise channel
          |
          v
Enterprise Agent Runtime ----> Ollama planner
   |       |       |
   |       |       +---------> PUL connector identity + receipts
   |       +-----------------> AGS authorization decisions
   +-------------------------> GovAgn traces and evidence
          |
          +----> SPACE Firecracker computer (files and code)
          |
          +----> dedicated Chromium sidecar (browser and screenshots)
```

## Product capabilities in this repository

- create persistent enterprise agent identities;
- give each active session a durable conversation and isolated computer;
- plan and execute bounded multi-step work with a local model;
- authorize SPACE launches and commands through AGS;
- use PUL-scoped connector identities instead of exposing provider secrets;
- capture tool evidence, screenshots, decision references, receipts, and traces;
- reject model completion claims that lack observable tool evidence;
- bind every local web/debugging surface to loopback.

## Current local-browser boundary

The agent's file and code computer is a real SPACE Firecracker microVM. The
browser is currently a dedicated Docker sidecar because the validated WSL
kernel does not provide the nftables transparent-socket capability required by
SPACE's template-build egress gateway. The runtime and UI identify these as
separate execution planes. Moving Chromium into SPACE requires a compatible
kernel followed by a successful browser-template build and CDP proxy proof.

## Production work still required

- enterprise OIDC and tenant isolation on the agent UI/API;
- encrypted secret storage and managed connector onboarding;
- durable background queues, schedules, retries, and human approval resumes;
- explicit browser domain and action policies;
- horizontally scalable state instead of local SQLite;
- evaluations for task success, prompt injection, and policy bypass;
- signed release artifacts and a supported installer.
