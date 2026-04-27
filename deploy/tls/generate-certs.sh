#!/bin/bash
set -e

# Certificate generation script for AgentFabric mTLS
# Generates self-signed CA and certificates for collector and agents

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "🔐 Generating AgentFabric mTLS certificates..."

# Generate CA private key
echo "1. Generating CA private key..."
openssl genrsa -out ca.key 2048 2>/dev/null

# Generate CA certificate (valid for 10 years)
echo "2. Generating CA certificate..."
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt \
  -subj "/CN=AgentFabric-CA/O=AgentFabric/C=US" 2>/dev/null

# Generate collector server private key
echo "3. Generating collector server key..."
openssl genrsa -out collector-server.key 2048 2>/dev/null

# Generate collector server certificate signing request
echo "4. Generating collector CSR..."
openssl req -new -key collector-server.key -out collector-server.csr \
  -subj "/CN=collector/O=AgentFabric/C=US" 2>/dev/null

# Sign collector certificate with CA (valid for 1 year)
echo "5. Signing collector certificate..."
openssl x509 -req -days 365 -in collector-server.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out collector-server.crt 2>/dev/null

# Generate agent client private key
echo "6. Generating agent client key..."
openssl genrsa -out agent-client.key 2048 2>/dev/null

# Generate agent client certificate signing request
echo "7. Generating agent CSR..."
openssl req -new -key agent-client.key -out agent-client.csr \
  -subj "/CN=agent-client/O=AgentFabric/C=US" 2>/dev/null

# Sign agent certificate with CA
echo "8. Signing agent certificate..."
openssl x509 -req -days 365 -in agent-client.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out agent-client.crt 2>/dev/null

# Clean up CSRs
rm -f *.csr ca.srl

# Set secure permissions
chmod 600 *.key
chmod 644 *.crt

echo ""
echo "✅ Certificates generated successfully!"
echo ""
echo "📋 Certificate files:"
echo "  CA:"
echo "    - ca.crt (CA certificate, share with all clients)"
echo "    - ca.key (CA private key, keep secure)"
echo ""
echo "  Collector (server):"
echo "    - collector-server.crt (server certificate)"
echo "    - collector-server.key (server private key)"
echo ""
echo "  Agent (client):"
echo "    - agent-client.crt (client certificate)"
echo "    - agent-client.key (client private key)"
echo ""
echo "📝 Certificate Info:"
openssl x509 -in ca.crt -text -noout | grep -E "Subject:|Issuer:|Not Before|Not After"
echo ""
echo "🔧 Next steps:"
echo "  1. Mount ca.crt, collector-server.crt, and collector-server.key in collector"
echo "  2. Mount ca.crt, agent-client.crt, and agent-client.key in agent SDKs"
echo "  3. Set environment variables:"
echo "     - Collector: GV_TLS_ENABLED=true GV_TLS_CERT_FILE=collector-server.crt GV_TLS_KEY_FILE=collector-server.key GV_TLS_MUTUAL_TLS=true GV_TLS_CLIENT_CA=ca.crt"
echo "     - Agent: Configure SDK with client cert/key for mTLS handshake"
echo ""
