# AgentKraft Enterprise Agent

Local, loopback-only enterprise agent runtime with:

- persistent agent identities and conversation history;
- a dedicated SPACE Firecracker computer for governed file and code actions;
- AGS authorization for every SPACE launch and command;
- PUL application identity and connector receipts;
- GovAgn/OpenTelemetry traces;
- a dedicated, persistent Chromium sidecar controlled through Playwright;
- screenshots and page observations recorded as task evidence.

## Run

The local AGS, PUL, GovAgn, Ollama, SPACE, Docker, and Ubuntu-24.04 WSL services
must already be available. From the AgentFabric repository, use the unified
Windows launcher:

```powershell
.\deploy\local-enterprise\start.ps1 -PulPackagePath C:\path\to\PUL -GovAgnPath C:\path\to\govagn
```

For direct Linux development, install `requirements.txt` into `.venv`, set the
required local service credentials, then run:

```bash
./start.sh
```

Open <http://127.0.0.1:8090>. Stop the web service with `./stop.sh`.

## Security boundary

This development build binds the UI and browser-debugging port to loopback. It
does not provide production user authentication. Browser navigation runs in a
per-session Docker sidecar because this machine's WSL kernel currently lacks
the nftables transparent-socket feature required by SPACE's template-build
network gateway. Code and file tools continue to run in real SPACE Firecracker
microVMs. The UI labels both planes rather than presenting the sidecar as a
Firecracker browser.
