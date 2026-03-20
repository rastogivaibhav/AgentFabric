#!/usr/bin/env bash
# install-ca-cert.sh — Install the AgentFabric NetProxy CA certificate
# into the OS trust store so transparent HTTPS interception is trusted by
# curl, Python, Node.js, Go, and any other tool using system root certs.
#
# Usage:
#   ./scripts/install-ca-cert.sh [GATEWAY_URL]
#
# GATEWAY_URL defaults to http://localhost:8080
#
# After installation, configure your tools to use the proxy:
#   export HTTP_PROXY=http://localhost:8443
#   export HTTPS_PROXY=http://localhost:8443
#   export http_proxy=http://localhost:8443
#   export https_proxy=http://localhost:8443
#
# For Python specifically (if REQUESTS_CA_BUNDLE is needed):
#   export REQUESTS_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt  # Linux
#   export REQUESTS_CA_BUNDLE=/etc/ssl/cert.pem                    # macOS

set -euo pipefail

GATEWAY_URL="${1:-http://localhost:8080}"
CERT_URL="${GATEWAY_URL}/api/v1/netproxy/ca.crt"
CERT_FILE="/tmp/agentfabric-netproxy-ca.crt"

echo "[af-netproxy] Fetching CA cert from ${CERT_URL} ..."
if ! curl -fsSL -o "${CERT_FILE}" "${CERT_URL}"; then
  echo "[af-netproxy] ERROR: could not fetch CA cert."
  echo "[af-netproxy] Is the api-gateway running at ${GATEWAY_URL}?"
  exit 1
fi

echo "[af-netproxy] CA cert saved to ${CERT_FILE}"

OS="$(uname -s)"
case "${OS}" in
  Linux*)
    if command -v update-ca-certificates &>/dev/null; then
      # Debian / Ubuntu / Alpine (ca-certificates package)
      echo "[af-netproxy] Installing into /usr/local/share/ca-certificates/ (Debian/Ubuntu)..."
      sudo cp "${CERT_FILE}" /usr/local/share/ca-certificates/agentfabric-netproxy.crt
      sudo update-ca-certificates
    elif command -v update-ca-trust &>/dev/null; then
      # RHEL / CentOS / Fedora
      echo "[af-netproxy] Installing into /etc/pki/ca-trust/source/anchors/ (RHEL/CentOS)..."
      sudo cp "${CERT_FILE}" /etc/pki/ca-trust/source/anchors/agentfabric-netproxy.crt
      sudo update-ca-trust extract
    else
      echo "[af-netproxy] ERROR: cannot detect Linux CA trust store tooling."
      echo "[af-netproxy] Manually add ${CERT_FILE} to your system trust store."
      exit 1
    fi
    ;;

  Darwin*)
    echo "[af-netproxy] Installing into macOS System Keychain (requires sudo)..."
    sudo security add-trusted-cert \
      -d -r trustRoot \
      -k /Library/Keychains/System.keychain \
      "${CERT_FILE}"
    ;;

  *)
    echo "[af-netproxy] ERROR: unsupported OS: ${OS}"
    echo "[af-netproxy] On Windows, import ${CERT_FILE} via:"
    echo "  certutil -addstore Root <path-to-cert>"
    echo "or via Settings → Certificate Manager → Trusted Root Certification Authorities."
    exit 1
    ;;
esac

echo ""
echo "[af-netproxy] ✓ CA cert installed successfully."
echo ""
echo "[af-netproxy] Set these environment variables to route LLM traffic through AgentFabric:"
echo ""
echo "  export HTTP_PROXY=http://localhost:8443"
echo "  export HTTPS_PROXY=http://localhost:8443"
echo "  export http_proxy=http://localhost:8443"
echo "  export https_proxy=http://localhost:8443"
echo ""
echo "[af-netproxy] Any process with these vars set will have its LLM API calls"
echo "[af-netproxy] observed in the AgentFabric portal (http://localhost:3000)."
