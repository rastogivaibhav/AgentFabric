# AgentFabric

> Governed, self-hosted AI teammates with isolated computers, policy-controlled
> actions, audit evidence, observability, and cost controls.

AgentFabric is an enterprise agent platform for organisations that want AI
teammates they can operate inside their own environment. It combines an agent
runtime with the existing AgentFabric observability and FinOps stack.

The current repository contains a working local development proof. An agent can
keep a conversation, plan multi-step work with a local model, use an isolated
SPACE computer, browse through a dedicated Chromium sidecar, and emit policy
decisions, connector receipts, screenshots, and traces.

AgentFabric is **not yet a production-ready replacement for Grok Bot, Rakazo,
OpenMausBot, OpenBot, or Hermes Agent**. Authentication, tenant isolation,
durable background execution, schedules, channels, managed connections, and
full browser governance remain on the roadmap.

## Why AgentFabric

Most AI teammate products start with chat and add controls later. AgentFabric is
being built from the opposite direction: every useful agent should have a
governed identity, a constrained computer, a record of what it did, and an
operator who can understand cost and risk.

The intended enterprise contract is:

- **Own the deployment** — run models, data, policy, and execution planes in
  infrastructure you control.
- **Isolate execution** — give each active agent session a SPACE Firecracker
  microVM for file and code work.
- **Authorize actions** — evaluate SPACE launches and commands through AGS.
- **Broker connections** — use PUL application identities and receipts rather
  than exposing provider credentials directly to an agent.
- **Prove what happened** — preserve decisions, process identifiers, tool
  output, screenshots, and OpenTelemetry traces.
- **Control cost** — attribute model and tool activity to agents and tenants and
  enforce budgets through AgentFabric's observability plane.

## What works today

| Capability | Current state |
|---|---|
| Create named agents with instructions and a model | Working in the local web UI |
| Persistent conversations and task evidence | Working in local SQLite storage |
| Multi-step model planning | Working with a bounded local Ollama loop |
| Isolated file and code execution | Working through SPACE Firecracker microVMs |
| Policy decisions for SPACE launch and commands | Working through AGS |
| Connector identity and receipts | PUL identity works; the included GitHub search is still a dry run |
| Browser, screenshots, click, and type | Working in a per-session Docker Chromium sidecar |
| Traces and operational dashboards | Working through GovAgn/OpenTelemetry |
| Windows development environment | Working through Windows, WSL2, Ubuntu 24.04, Docker, and the local launcher |
| Production identity and tenant isolation | Not implemented in the enterprise-agent runtime |
| Crash-safe background jobs, retries, and schedules | Not implemented |
| Slack, Teams, email, mobile, or webhook channels | Not implemented |
| Human approval pause and resume | Not implemented |
| Long-term semantic memory and agent-to-agent delegation | Not implemented |

## Architecture

```text
                           Enterprise user / future channels
                                          |
                                          v
                         +----------------------------------+
                         | AgentFabric Agent Runtime        |
                         | identity · conversation · planner|
                         +--------+--------------+----------+
                                  |              |
                         plan     |              | telemetry
                                  v              v
                              Ollama         GovAgn / OTel
                                  |
                                  v
                  +---------------+------+----------------+
                  |                      |                |
                  v                      v                v
             AGS permit              PUL receipt    Docker Chromium
                  |                      |           browser · screenshots
                  v                      v           governance pending
          SPACE MissionBroker      Enterprise apps
          Firecracker computer     and data sources
          files · code · shell
                  |
                  +------------- evidence -------------> GovAgn
```

The browser is intentionally shown as a separate execution plane. On the
validated WSL2 machine, SPACE's template-build network gateway could not host
Chromium because the kernel lacked the required nftables transparent-socket
capability. File and code tools run in SPACE; browser actions currently run in
a dedicated Docker sidecar.

## How AgentFabric compares

The projects below overlap, but they optimise for different buyers. This table
describes the documented projects as of October 2026 and avoids treating a
roadmap item as a shipped feature.

| Area | AgentFabric / AgentKraft | [Rakazo](https://github.com/elie222/rakazo) | [OpenMausBot](https://github.com/milind-soni/OpenMausBot) | [OpenBot](https://github.com/CopilotKit/OpenBot) | [Hermes Agent](https://github.com/NousResearch/hermes-agent) |
|---|---|---|---|---|---|
| Primary focus | Governed enterprise agents plus observability and FinOps | Self-hosted persistent AI teammates | Local-first desktop team of CLI-backed bots | Enterprise coworker template with a governed tool gateway | Extensible personal agent, tools, skills, and automation |
| Repository licence | No `LICENSE` file; README declares proprietary | Apache-2.0 | Apache-2.0 | MIT | MIT |
| Main interface | Local web agent UI plus observability portal | Web, Electron desktop, iOS, Android | Desktop/web workspace with companion access | Web workspace | CLI, TUI, web dashboard, and messaging gateways |
| Models | Local Ollama in the current runtime; broader models through the existing ecosystem | Bring your own model through Pi and supported providers | Claude, Codex, Grok, custom CLIs, and API engines | Any AG-UI agent; examples use common model providers | Multiple hosted and OpenAI-compatible providers |
| Agent computers | SPACE Firecracker for code; Docker Chromium for browser | Docker and several local or hosted computer providers | Cloud computer, local VM, or explicitly enabled host control | One supervised container and Chromium profile per coworker | Local, Docker, SSH, Singularity, and cloud backends |
| Persistent agents | Conversation and agent records persist; turns are not yet crash-safe | Conversations, memory, routines, and history | Separate bot threads, folders, computers, and connected apps | Durable coworker profiles and channels | Sessions, memory, skills, and persistent sandbox options |
| Schedules | Roadmap | Shipped routines | Shipped recurring work | Platform-dependent | Shipped cron tools |
| Channels | Roadmap | Web, desktop, and mobile clients | Desktop/web plus connected apps and automation interfaces | Built around channels and AG-UI | Telegram, Discord, Slack, WhatsApp, Signal, email, Teams, and others |
| Delegation | Roadmap | Peer bots and short-lived subagents | Team orchestration and task assignment | Depends on the connected AG-UI agent | Built-in delegation tools |
| Governance | AGS permits, PUL receipts, tamper-evident mission journal, budgets, and traces; browser policy is incomplete | Approvals, audit history, and operator-owned infrastructure | Permission broker and explicit approval flows | Central policy gateway and PostgreSQL audit log | Tool controls, approvals, pairing, and hardened container modes |
| Observability and FinOps | Core product capability | Product activity and audit focus | Runtime event stream and operational UI | Audit and OpenTelemetry integration | Logs, sessions, status, and extensible tooling |
| Current maturity | Integrated local development proof; production hardening remains | Project describes itself as beta | Actively released end-user product | Project describes itself as an alpha template | Broad, actively developed agent framework |

### Where AgentFabric is differentiated

AgentFabric should not try to win by copying every consumer interface first.
Its strongest potential position is an enterprise control plane for AI
teammates:

1. **Governance before execution.** AGS decisions are tied to SPACE missions,
   and denied or ambiguous operations fail closed.
2. **MicroVM isolation.** Code execution uses a real SPACE Firecracker plane
   rather than sharing the web server or relying only on a general container.
3. **Evidence across the full task.** Policy decisions, connector receipts,
   process results, screenshots, and traces can be joined into one audit story.
4. **Observability and FinOps in the same platform.** Agent traces, costs,
   budgets, and policy outcomes are first-class rather than add-ons.
5. **Private deployment.** The architecture is aimed at local, on-premise, and
   air-gapped environments.

### Where the alternatives are ahead

Rakazo and OpenMausBot currently provide a much more complete AI teammate
experience: polished rosters, broad model choice, schedules, connected apps,
multiple clients, and practical onboarding. Hermes has a large tool, skill,
memory, channel, and sandbox surface. OpenBot has a clearer single gateway for
all browser, file, computer, and MCP actions.

AgentFabric must close these gaps before claiming feature parity:

1. Authentication, SSO, tenant ownership, and role-based access.
2. A durable task coordinator with leases, checkpoints, retries, cancellation,
   and approval pause/resume.
3. One governed action gateway for browser, computer, file, connector, and MCP
   operations.
4. Real managed connections and OAuth onboarding instead of dry-run connectors.
5. Schedules, webhooks, Slack/Teams channels, notifications, and streaming task
   progress.
6. Searchable long-term memory, reusable skills, delegation, and evaluation
   suites.
7. Production packaging, signed images, upgrades, backup/restore, and supportable
   installers.

## Local quick start

### Prerequisites

- Windows 11 with WSL2 and Ubuntu 24.04
- Docker Desktop with WSL integration
- A bootstrapped local SPACE cluster
- Local AGS, PUL, GovAgn, and Ollama services
- Python 3.10 or newer inside WSL

### Start the consolidated stack

From PowerShell:

```powershell
$env:SPACE_LOCAL_PASSWORD = '<your-local-space-password>'
.\deploy\local-enterprise\start.ps1 `
  -PulPackagePath C:\path\to\PUL `
  -GovAgnPath C:\path\to\govagn
```

Open the agent workspace at <http://127.0.0.1:8090/>.

The launcher binds development services to loopback. Loopback is not a
substitute for authentication; do not expose this profile to another machine
or an untrusted network.

### Verify the governed provider contracts

Inside WSL:

```bash
python3 -m unittest discover \
  -s integrations/space_provider \
  -p 'test_*.py' \
  -v
```

The current contract suite contains 50 tests covering the SPACE client,
mission journal, AGS authorization adapter, and PUL client. These fixture tests
do not constitute Firecracker proof; a real microVM claim requires a successful
launch, command, evidence check, and teardown on a KVM-capable host.

## Repository layout

```text
enterprise-agent/             Local persistent agent runtime and web UI
integrations/space_provider/  SPACE, AGS, PUL, and mission-journal adapters
deploy/local-enterprise/      Windows/WSL consolidated launcher
agent-sdk/                    Python agent auto-instrumentation
collector/                    OpenTelemetry ingestion and enrichment
api-gateway/                  Authentication, APIs, budgets, and live events
af-core/                      Policy evaluation and analytics pipeline
portal/                       Enterprise observability portal
deploy/                       Docker, Kubernetes, Helm, and database assets
docs/                         Architecture, operations, and proof boundaries
```

## Production roadmap

### Phase 1 — Secure control plane

- OIDC/SSO, tenant ownership, RBAC, and protected artifacts
- split privileged provisioning from the user-facing runtime
- governed browser policy, network egress controls, and content-bound permits
- PostgreSQL state and encrypted secret storage

### Phase 2 — Durable agent runtime

- background workers with checkpoints and renewable leases
- idempotent provisioning and reconciliation
- human approval pause/resume, cancellation, and task replay
- quotas, retention, and evidence lifecycle management

### Phase 3 — Enterprise teammate experience

- managed OAuth connections and real PUL connector results
- schedules, notifications, Slack, Teams, email, and webhooks
- reusable skills, long-term memory, and controlled delegation
- live task streaming and operator takeover

### Phase 4 — Release confidence

- task-success, prompt-injection, and policy-bypass evaluations
- load, failure-recovery, and multi-tenant isolation tests
- pinned dependencies, signed images, SBOMs, installers, and upgrades
- backup, restore, disaster recovery, and operational runbooks

## Security status

The `enterprise-agent` application is a local development profile. It currently
has no production user authentication, uses a single hard-coded tenant, stores
state locally, and performs browser actions outside the AGS command path. It
also needs privileged credentials to provision its local dependencies.

Treat it as an engineering proof, not as a production security boundary. See
[Enterprise Agent Runtime](docs/enterprise-agent-runtime.md) and
[SPACE Provider Integration](integrations/space_provider/README.md) for the
verified boundaries and explicit blockers.

## Source notes for the comparison

- [Rakazo README](https://github.com/elie222/rakazo) and
  [self-hosting guide](https://github.com/elie222/rakazo/blob/main/docs/self-host.md)
- [OpenMausBot README](https://github.com/milind-soni/OpenMausBot) and
  [self-hosting guide](https://github.com/milind-soni/OpenMausBot/blob/main/docs/self-hosting.md)
- [OpenBot README](https://github.com/CopilotKit/OpenBot),
  [coworker model](https://github.com/CopilotKit/OpenBot/blob/main/docs/coworkers.md), and
  [configuration](https://github.com/CopilotKit/OpenBot/blob/main/docs/configuration.md)
- [Hermes Agent README](https://github.com/NousResearch/hermes-agent) and
  [quick start](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/getting-started/quickstart.md)

## License

This repository has no root `LICENSE` file. Its existing project documentation
declares AgentFabric proprietary and directs licensing questions to
[team@agentfabric.io](mailto:team@agentfabric.io).

That licence is a material difference from the open-source alternatives above.
If AgentFabric is intended to compete as an open-source Grok Bot alternative,
the project must adopt and add an explicit OSI-approved `LICENSE` file before
making that claim.
