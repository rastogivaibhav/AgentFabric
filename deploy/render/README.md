# Render Deployment

Govagn can run on Render as a design-partner deployment with:

- `govagn-gateway`: Docker web service for the Go API gateway
- `govagn-portal`: Render static site for the React portal
- `govagn-db`: Render Postgres
- `govagn-redis`: Render Key Value

The default Render blueprint intentionally disables Kafka with `GV_KAFKA_ENABLED=false`. This is enough for the governed release workflow, prompt/eval/policy/rollout controls, and audit evidence. Add Kafka-compatible infrastructure later if you need durable async ingest parity with local Docker Compose.

## Deploy

1. Push this repo to GitHub.
2. In Render, create a new Blueprint from the repo.
3. Use the root-level `render.yaml`.
4. During setup, provide:
   - `GV_ADMIN_PASSWORD`: initial admin password
   - `GV_VAULT_KEY`: 64 hex characters for provider-key encryption
5. Deploy.

You can generate a local vault key with PowerShell:

```powershell
-join ((1..64) | ForEach-Object { '{0:x}' -f (Get-Random -Minimum 0 -Maximum 16) })
```

## Expected URLs

The blueprint assumes these Render subdomains:

```text
https://govagn-gateway.onrender.com
https://govagn-portal.onrender.com
```

If Render changes either subdomain because of a name collision, update:

- `VITE_API_URL` on `govagn-portal`
- `GV_CORS_ORIGINS` on `govagn-gateway`
- optional OIDC redirect settings if you enable SSO

## Scope

This deployment is suitable for a weekend design-partner demo. It is not the final hardened enterprise topology.

Before production, tighten:

- SSO/OIDC and password login policy
- custom domains
- CORS origins
- backup and restore policy
- persistent NetProxy CA configuration if using transparent proxying
- Kafka or another durable event queue for async ingest
- collector deployment for public OTLP ingest
