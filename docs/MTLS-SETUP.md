# mTLS (Mutual TLS) Setup Guide for Production

## Overview

This guide enables mutual TLS authentication between AgentFabric components in production to ensure encrypted, authenticated communication.

---

## Prerequisites

- OpenSSL 1.1.1+
- kubectl with admin access
- Cert-Manager installed in cluster (recommended)
- 1-2 hours for initial setup

---

## 1. Generate CA Certificates

### Option A: Using OpenSSL (Recommended for small deployments)

```bash
# Create CA private key
openssl genrsa -out ca-key.pem 4096

# Create CA certificate
openssl req -new -x509 -days 3650 -key ca-key.pem -out ca-cert.pem \
  -subj "/CN=govagn-ca/O=AgentFabric/C=US"

# Store in Kubernetes secret
kubectl create secret tls govagn-ca \
  --cert=ca-cert.pem \
  --key=ca-key.pem \
  -n govagn
```

### Option B: Using Cert-Manager (Recommended for large deployments)

```bash
# Install cert-manager (if not already installed)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.yaml

# Create ClusterIssuer for CA
kubectl apply -f - << 'EOF'
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: govagn-selfsigned
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: govagn-ca
  namespace: govagn
spec:
  isCA: true
  commonName: govagn-ca
  secretName: govagn-ca
  issuerRef:
    name: govagn-selfsigned
    kind: ClusterIssuer
---
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: govagn-issuer
spec:
  ca:
    secretName: govagn-ca
EOF
```

---

## 2. Generate Component Certificates

### Collector Service Certificate

```bash
# Create certificate request
cat > collector-csr.json << 'EOF'
{
  "CN": "govagn-collector",
  "hosts": [
    "govagn-collector",
    "govagn-collector.govagn",
    "govagn-collector.govagn.svc",
    "govagn-collector.govagn.svc.cluster.local"
  ],
  "key": {
    "algo": "rsa",
    "size": 2048
  }
}
EOF

# Using cfssl (recommended)
cfssl gencert -ca=ca-cert.pem -ca-key=ca-key.pem collector-csr.json | cfssljson -bare collector

# OR using OpenSSL
openssl req -new -keyout collector-key.pem -out collector.csr \
  -subj "/CN=govagn-collector/O=AgentFabric/C=US"

openssl x509 -req -days 365 -in collector.csr \
  -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial \
  -out collector-cert.pem \
  -extensions "subjectAltName=DNS:govagn-collector,DNS:govagn-collector.govagn.svc.cluster.local"

# Create secret
kubectl create secret tls govagn-collector-tls \
  --cert=collector-cert.pem \
  --key=collector-key.pem \
  -n govagn
```

### API Gateway Service Certificate

```bash
# Similar process for API Gateway
openssl req -new -keyout api-gateway-key.pem -out api-gateway.csr \
  -subj "/CN=govagn-api-gateway/O=AgentFabric/C=US"

openssl x509 -req -days 365 -in api-gateway.csr \
  -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial \
  -out api-gateway-cert.pem

kubectl create secret tls govagn-api-gateway-tls \
  --cert=api-gateway-cert.pem \
  --key=api-gateway-key.pem \
  -n govagn
```

### Create CA Certificate Secret for Client Verification

```bash
kubectl create secret generic govagn-ca-cert \
  --from-file=ca.pem=ca-cert.pem \
  -n govagn
```

---

## 3. Configure Collector for mTLS

Update `collector/cmd/collector/main.go`:

```go
package main

import (
	"crypto/tls"
	"crypto/x509"
	"os"
)

func initTLS() (*tls.Config, error) {
	// Load CA certificate
	caCert, err := os.ReadFile("/etc/certs/ca/ca.pem")
	if err != nil {
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Load server certificate and key
	cert, err := tls.LoadX509KeyPair(
		"/etc/certs/tls/tls.crt",
		"/etc/certs/tls/tls.key",
	)
	if err != nil {
		return nil, err
	}

	// Create TLS configuration
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}

	return tlsConfig, nil
}

func main() {
	// Initialize mTLS
	tlsConfig, err := initTLS()
	if err != nil {
		log.Fatalf("Failed to initialize TLS: %v", err)
	}

	// Create gRPC server with mTLS
	grpcServer := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConfig)),
	)

	// ... rest of initialization
}
```

---

## 4. Configure API Gateway for mTLS

Update `api-gateway/cmd/server/main.go`:

```go
// Similar TLS initialization
func initTLS() (*tls.Config, error) {
	// Load client certificate for outbound connections
	cert, err := tls.LoadX509KeyPair(
		"/etc/certs/tls/tls.crt",
		"/etc/certs/tls/tls.key",
	)
	if err != nil {
		return nil, err
	}

	caCert, err := os.ReadFile("/etc/certs/ca/ca.pem")
	if err != nil {
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
```

---

## 5. Update Kubernetes Deployment

### Patch Collector Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: govagn-collector
  namespace: govagn
spec:
  template:
    spec:
      containers:
      - name: collector
        volumeMounts:
        - name: tls-certs
          mountPath: /etc/certs/tls
          readOnly: true
        - name: ca-cert
          mountPath: /etc/certs/ca
          readOnly: true
        env:
        - name: TLS_ENABLED
          value: "true"
        - name: TLS_CERT_PATH
          value: "/etc/certs/tls/tls.crt"
        - name: TLS_KEY_PATH
          value: "/etc/certs/tls/tls.key"
        - name: TLS_CA_PATH
          value: "/etc/certs/ca/ca.pem"
      volumes:
      - name: tls-certs
        secret:
          secretName: govagn-collector-tls
      - name: ca-cert
        secret:
          secretName: govagn-ca-cert
```

### Apply via Helm

```bash
helm upgrade govagn ./deploy/helm \
  -n govagn \
  --set collector.tls.enabled=true \
  --set apiGateway.tls.enabled=true \
  --set tls.caSecret=govagn-ca-cert
```

---

## 6. Verification

### Test mTLS Connection

```bash
# From inside API Gateway pod
kubectl exec -it govagn-api-gateway-0 -n govagn -- bash

# Test connection to Collector with mTLS
curl --cacert /etc/certs/ca/ca.pem \
     --cert /etc/certs/tls/tls.crt \
     --key /etc/certs/tls/tls.key \
     https://govagn-collector:4317/v1/traces
```

### Check Certificate Validity

```bash
# View certificate details
kubectl get secret govagn-collector-tls -n govagn -o jsonpath='{.data.tls\.crt}' | \
  base64 -d | openssl x509 -text -noout

# Verify expiration dates
openssl x509 -in collector-cert.pem -noout -dates
```

### Monitor TLS Metrics

```bash
# Check Prometheus for TLS errors
kubectl port-forward -n govagn svc/prometheus 9090:9090

# Query: increase(govagn_tls_handshake_errors_total[5m])
```

---

## 7. Certificate Rotation

### Automated Rotation (Recommended)

Using Cert-Manager, certificates auto-renew 30 days before expiration:

```bash
# Monitor renewal status
kubectl describe certificate govagn-collector -n govagn
```

### Manual Rotation

```bash
# Generate new certificate
openssl req -new -keyout collector-key-new.pem -out collector-new.csr \
  -subj "/CN=govagn-collector/O=AgentFabric/C=US"

openssl x509 -req -days 365 -in collector-new.csr \
  -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial \
  -out collector-cert-new.pem

# Update secret
kubectl create secret tls govagn-collector-tls \
  --cert=collector-cert-new.pem \
  --key=collector-key-new.pem \
  -n govagn \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart pods to pick up new certificate
kubectl rollout restart deployment/govagn-collector -n govagn
```

---

## 8. Troubleshooting

### TLS Handshake Failures

```bash
# Check logs
kubectl logs -n govagn deployment/govagn-collector | grep -i "tls\|error"

# Verify certificate chain
openssl verify -CAfile ca-cert.pem collector-cert.pem

# Check hostname matches
openssl x509 -in collector-cert.pem -noout -text | grep -A 5 "Subject Alternative Name"
```

### Certificate Expiration

```bash
# Check remaining validity
openssl x509 -in collector-cert.pem -noout -dates

# If expired, regenerate and rotate immediately
```

### Connection Refused

```bash
# Verify secrets exist
kubectl get secrets -n govagn | grep tls

# Check volume mounts
kubectl describe pod -n govagn $(kubectl get pods -n govagn -l app=govagn-collector -o name | head -1)
```

---

## Security Best Practices

✅ Use TLS 1.3 minimum (enforced in code)
✅ Rotate certificates every 90 days
✅ Use strong key sizes (2048-bit RSA or 256-bit ECDSA)
✅ Verify hostname in certificates
✅ Monitor certificate expiration
✅ Restrict secret access via RBAC
✅ Encrypt secrets at rest in etcd

---

**Last Updated:** 2026-05-04
**Maintained By:** Security Team
