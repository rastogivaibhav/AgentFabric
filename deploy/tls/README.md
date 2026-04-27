# mTLS Configuration for AgentFabric Collector

This guide covers mutual TLS (mTLS) setup for the collector, preventing unauthorized span injection and securing the telemetry pipeline.

## Overview

mTLS requires:
- **Collector** authenticates with a server certificate
- **Agents/SDKs** authenticate with client certificates
- Both sides verify the peer's certificate against a trusted CA

## Quick Start

### 1. Generate Certificates

```bash
cd deploy/tls
bash generate-certs.sh
```

This creates:
- `ca.crt` / `ca.key` — Certificate Authority
- `collector-server.crt` / `collector-server.key` — Collector server
- `agent-client.crt` / `agent-client.key` — Agent client

### 2. Configure Collector

Set environment variables:

```bash
# Enable TLS
GV_TLS_ENABLED=true

# Server certificate and key
GV_TLS_CERT_FILE=/etc/govagn/tls/collector-server.crt
GV_TLS_KEY_FILE=/etc/govagn/tls/collector-server.key

# Enable mutual TLS (require client certs)
GV_TLS_MUTUAL_TLS=true

# CA to verify client certificates
GV_TLS_CLIENT_CA=/etc/govagn/tls/ca.crt
```

### 3. Configure Agents

For Python SDK:

```python
import ssl
from anthropic import Anthropic

# Create SSL context with client certificate
ssl_context = ssl.create_default_context()
ssl_context.load_cert_chain(
    certfile='/path/to/agent-client.crt',
    keyfile='/path/to/agent-client.key',
    password=None
)
ssl_context.load_verify_locations('/path/to/ca.crt')

# Configure to use SSL
client = Anthropic(
    api_key='...',
    # Note: SDK integration with custom SSL context depends on implementation
)
```

For JavaScript/Node.js:

```javascript
const https = require('https');
const fs = require('fs');
const tls = require('tls');

const options = {
  key: fs.readFileSync('/path/to/agent-client.key'),
  cert: fs.readFileSync('/path/to/agent-client.crt'),
  ca: fs.readFileSync('/path/to/ca.crt'),
  rejectUnauthorized: true,
};

const client = https.createSecureContext(options);
// Use client in HTTP/gRPC client configuration
```

## Docker Compose

Mount certificates:

```yaml
collector:
  volumes:
    - ./deploy/tls/ca.crt:/etc/govagn/tls/ca.crt:ro
    - ./deploy/tls/collector-server.crt:/etc/govagn/tls/collector-server.crt:ro
    - ./deploy/tls/collector-server.key:/etc/govagn/tls/collector-server.key:ro
  environment:
    GV_TLS_ENABLED: "true"
    GV_TLS_MUTUAL_TLS: "true"
    GV_TLS_CERT_FILE: /etc/govagn/tls/collector-server.crt
    GV_TLS_KEY_FILE: /etc/govagn/tls/collector-server.key
    GV_TLS_CLIENT_CA: /etc/govagn/tls/ca.crt
```

## Kubernetes

Create secrets:

```bash
kubectl create secret tls collector-server \
  --cert=deploy/tls/collector-server.crt \
  --key=deploy/tls/collector-server.key

kubectl create secret generic collector-ca \
  --from-file=ca.crt=deploy/tls/ca.crt
```

Mount in pod:

```yaml
containers:
  - name: collector
    volumeMounts:
      - name: server-tls
        mountPath: /etc/govagn/tls/server
        readOnly: true
      - name: client-ca
        mountPath: /etc/govagn/tls/client
        readOnly: true
    env:
      - name: GV_TLS_ENABLED
        value: "true"
      - name: GV_TLS_MUTUAL_TLS
        value: "true"
      - name: GV_TLS_CERT_FILE
        value: /etc/govagn/tls/server/tls.crt
      - name: GV_TLS_KEY_FILE
        value: /etc/govagn/tls/server/tls.key
      - name: GV_TLS_CLIENT_CA
        value: /etc/govagn/tls/client/ca.crt
volumes:
  - name: server-tls
    secret:
      secretName: collector-server
  - name: client-ca
    secret:
      secretName: collector-ca
```

## Testing

Test mTLS with OpenSSL:

```bash
# Test successful connection (with client cert)
openssl s_client \
  -connect localhost:4317 \
  -cert deploy/tls/agent-client.crt \
  -key deploy/tls/agent-client.key \
  -CAfile deploy/tls/ca.crt

# Test failure (no client cert)
# Should be refused with certificate verification error
openssl s_client \
  -connect localhost:4317 \
  -CAfile deploy/tls/ca.crt
```

## Certificate Rotation

To rotate certificates:

1. Generate new certificates with same CA:
   ```bash
   openssl genrsa -out collector-server-new.key 2048
   openssl req -new -key collector-server-new.key -out collector-server-new.csr \
     -subj "/CN=collector/O=AgentFabric/C=US"
   openssl x509 -req -days 365 -in collector-server-new.csr \
     -CA ca.crt -CAkey ca.key -CAcreateserial \
     -out collector-server-new.crt
   ```

2. Update secrets:
   ```bash
   kubectl create secret tls collector-server \
     --cert=collector-server-new.crt \
     --key=collector-server-new.key \
     --dry-run=client -o yaml | kubectl apply -f -
   ```

3. Restart collector pods to pick up new secrets

## Troubleshooting

**"certificate verify failed"**
- Verify client certificate is signed by the CA
- Check CA certificate is loaded in client SSL context
- Ensure certificate paths are correct

**"certificate required"**
- Client is not providing a certificate
- Set mTLS requirement correctly
- Verify agent SDK configuration includes client cert

**"certificate unknown"**
- Client certificate not signed by collector's CA
- CA changed and certificates not regenerated
- Certificate CN doesn't match expected identity

**Port in use or connection refused**
- Collector may not have started due to cert issue
- Check collector logs: `docker logs <collector-container>`
- Verify file paths and permissions on certificates

## Security Best Practices

1. **Keep CA key secure** — Never commit `ca.key` to git
2. **Rotate certificates regularly** — Set to 1 year expiration
3. **Use separate CAs for dev/staging/prod** — Don't mix environments
4. **Validate certificate chain** — Verify issuer/subject on certificates
5. **Monitor certificate expiry** — Set alerts 30 days before expiration
6. **Restrict file permissions** — `chmod 600` on private keys

## References

- [OpenSSL Documentation](https://www.openssl.org/docs/)
- [gRPC Security](https://grpc.io/docs/guides/auth/)
- [Kubernetes TLS Secrets](https://kubernetes.io/docs/concepts/configuration/secret/#tls-secrets)
