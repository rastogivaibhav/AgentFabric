#!/usr/bin/env bash
# AgentFabric — generate self-signed mTLS certificates for local development.
#
# Usage:
#   ./scripts/generate-dev-certs.sh
#
# Creates:
#   deploy/certs/ca.crt        Root CA certificate
#   deploy/certs/ca.key        Root CA private key (keep secret)
#   deploy/certs/collector.crt Collector server certificate (signed by CA)
#   deploy/certs/collector.key Collector server private key
#   deploy/certs/client.crt    Client certificate for SDK / tests
#   deploy/certs/client.key    Client private key
#
# To enable mTLS in dev, set in docker-compose.yml:
#   AF_TLS_ENABLED: "true"
#   AF_TLS_CERT_FILE: /certs/collector.crt
#   AF_TLS_KEY_FILE:  /certs/collector.key
# And mount: ./deploy/certs:/certs:ro
#
# NEVER commit the generated *.key or *.crt files — they are in .gitignore.

set -euo pipefail

CERT_DIR="$(dirname "$0")/../deploy/certs"
mkdir -p "$CERT_DIR"
cd "$CERT_DIR"

DAYS=3650   # 10 years — dev only
C="US"
O="AgentFabric Dev"
CN_CA="AgentFabric Dev CA"
CN_COLLECTOR="collector.agentfabric.local"
CN_CLIENT="client.agentfabric.local"

echo "→ Generating development CA..."
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days "$DAYS" -key ca.key -out ca.crt \
  -subj "/C=${C}/O=${O}/CN=${CN_CA}"

echo "→ Generating collector server certificate..."
openssl genrsa -out collector.key 2048
openssl req -new -key collector.key -out collector.csr \
  -subj "/C=${C}/O=${O}/CN=${CN_COLLECTOR}"

cat > collector-ext.cnf <<EOF
[SAN]
subjectAltName=DNS:collector,DNS:collector.agentfabric.local,DNS:localhost,IP:127.0.0.1
EOF

openssl x509 -req -days "$DAYS" \
  -in collector.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out collector.crt \
  -extfile collector-ext.cnf -extensions SAN

echo "→ Generating client certificate..."
openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr \
  -subj "/C=${C}/O=${O}/CN=${CN_CLIENT}"
openssl x509 -req -days "$DAYS" \
  -in client.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out client.crt

echo "→ Cleaning up CSR and extension files..."
rm -f collector.csr client.csr collector-ext.cnf

# docker-compose.yml references /certs/server.crt and /certs/server.key.
# Create copies under those names so both the docker-compose default path and
# the explicit collector.* names work without changing docker-compose.
echo "→ Creating server.crt / server.key aliases for docker-compose..."
cp collector.crt server.crt
cp collector.key server.key

echo ""
echo "✓ Certificates written to: $(pwd)"
echo "  ca.crt         — CA certificate (share with clients)"
echo "  server.crt     — Server cert (alias for collector.crt; used by docker-compose)"
echo "  server.key     — Server key  (alias for collector.key; used by docker-compose)"
echo "  collector.crt  — Collector server cert"
echo "  collector.key  — Collector server key (KEEP SECRET)"
echo "  client.crt     — Client certificate"
echo "  client.key     — Client key (KEEP SECRET)"
echo ""
echo "To test mTLS locally:"
echo "  make dev-tls"
echo ""
echo "Or manually:"
echo "  AF_TLS_ENABLED=true docker compose up -d --build"
echo ""
echo "SDK mTLS example:"
echo "  from agentfabric import instrument"
echo "  instrument("
echo "      endpoint='https://localhost:4317',"
echo "      insecure=False,"
echo "  )"
