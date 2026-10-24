# Local enterprise-agent stack

This folder starts the complete local AgentFabric agent experience from this
single source repository. The product code lives here; runtime engines remain
external dependencies:

- AGS policy/control-plane containers;
- a bootstrapped SPACE cluster and `seed:latest` Firecracker template;
- the PUL companion distribution;
- GovAgn's Docker Compose deployment;
- Ollama with the configured local model;
- Docker Desktop and Ubuntu 24.04 on WSL2.

No token, database, PID, trace, screenshot, or browser profile is committed.
Local state is stored below `.local/enterprise-stack/`, which is ignored.

## Start on Windows

```powershell
$env:SPACE_LOCAL_PASSWORD = '<your local SPACE password>'
.\deploy\local-enterprise\start.ps1 `
  -PulPackagePath C:\path\to\PUL-XP-Companion-Linux-x86_64 `
  -GovAgnPath C:\path\to\govagn
```

Then run `status.ps1` and `open.ps1`. The agent workspace is available at
<http://127.0.0.1:8090>.

## Execution planes

Code and file actions run in dedicated SPACE Firecracker microVMs and pass
through AGS. Browser actions run in a per-session Playwright Chromium sidecar
bound to loopback and are labelled as such in the UI. This sidecar is the
current local fallback because the stock WSL kernel on the validated machine
lacks the nftables transparent-socket feature required by SPACE's template
builder. It must not be represented as a Firecracker-hosted browser.
