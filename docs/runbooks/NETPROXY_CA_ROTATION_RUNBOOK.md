# NetProxy CA Rotation Runbook

## Purpose

This runbook defines how operators back up, restore, and rotate the persisted NetProxy CA used by Govagn's transparent interception path.

Production deployments must use a stable persisted CA. Restarting the gateway must not silently mint a new trust root.

## What This Covers

- backup of the persisted NetProxy CA certificate and private key
- restore verification for the current release cycle
- planned rotation guidance
- release evidence required before GA or production changes

## Required Files

- persisted certificate file exposed to the gateway through `GV_NETPROXY_CA_CERT_FILE`
- persisted private key file exposed to the gateway through `GV_NETPROXY_CA_KEY_FILE`

For Docker Compose, these are mounted from:

- `GV_NETPROXY_CA_CERT_PATH`
- `GV_NETPROXY_CA_KEY_PATH`

For Helm, these are projected from the shared Secret configured by:

- `api.netproxyCA.certKey`
- `api.netproxyCA.keyKey`

## Backup and Restore Drill

Run one drill per release cycle and attach the generated markdown summary to the release review.

### Windows PowerShell

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\exercise_netproxy_ca_backup_restore.ps1 `
  -NetProxyCaCertFile C:\secure\govagn\netproxy-ca.crt `
  -NetProxyCaKeyFile C:\secure\govagn\netproxy-ca.key `
  -OutputPath .\netproxy-ca-drill.md
```

### macOS / Linux

```bash
NETPROXY_CA_CERT_FILE=/secure/govagn/netproxy-ca.crt \
NETPROXY_CA_KEY_FILE=/secure/govagn/netproxy-ca.key \
OUTPUT_PATH=./netproxy-ca-drill.md \
bash scripts/exercise-netproxy-ca-backup-restore.sh
```

The drill does not mutate the live files. It:

- copies the current CA cert and key into a timestamped backup directory
- restores those copies into a separate verification directory
- verifies hash equality
- verifies the restored certificate still matches the restored key in the shell workflow
- emits a markdown summary for release evidence

## Planned Rotation

Rotate only during a planned change window because clients that trust the old root must be updated to trust the new root before interception resumes cleanly.

Recommended sequence:

1. back up the current cert and key and capture the drill summary
2. generate the new root CA pair in a secure workspace
3. distribute the new CA cert to the required workload or device trust stores
4. update the persisted CA secret or mounted files
5. restart the gateway in a controlled window
6. validate `GET /healthz`, `GET /readyz`, proxy-path proof, and one live intercepted request
7. retain the prior CA backup until rollback is no longer required

## Emergency Restore

If a rotation fails or a new CA is not yet trusted:

1. restore the previous persisted cert and key from the timestamped backup
2. restart the gateway
3. verify the certificate fingerprint matches the expected prior root
4. rerun proxy-path proof and one candidate validation cycle

## Release Evidence

Before GA or a production rollout:

- the backup and restore drill summary must show `Validation result: PASS`
- the certificate fingerprint in the report must match the intended production CA
- the release review must note whether the cycle used the existing CA or performed a planned rotation

## Related Documents

Use this runbook with:

- [../BACKUP_RESTORE.md](../BACKUP_RESTORE.md)
- [../PRODUCTION_CHECKLIST.md](../PRODUCTION_CHECKLIST.md)
- [../REFERENCE_DEPLOYMENT.md](../REFERENCE_DEPLOYMENT.md)
