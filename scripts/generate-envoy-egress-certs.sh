#!/usr/bin/env bash
set -euo pipefail

# Generates per-domain MITM certificates for Envoy egress interception.
#
# Required:
#   deploy/certs/ca.crt
#   deploy/certs/ca.key
#
# Created:
#   deploy/certs/api.openai.com.crt
#   deploy/certs/api.openai.com.key
#   deploy/certs/api.anthropic.com.crt
#   deploy/certs/api.anthropic.com.key
#   deploy/certs/generativelanguage.googleapis.com.crt
#   deploy/certs/generativelanguage.googleapis.com.key
#
# Usage:
#   ./scripts/generate-envoy-egress-certs.sh
#
# Notes:
# - Workloads that are intercepted by Envoy must trust deploy/certs/ca.crt.
# - Do not commit generated keys/certs.

CERT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../deploy/certs" && pwd)"
CA_CRT="${CERT_DIR}/ca.crt"
CA_KEY="${CERT_DIR}/ca.key"
DAYS="${DAYS:-825}"

if [[ ! -f "${CA_CRT}" || ! -f "${CA_KEY}" ]]; then
  echo "Missing CA files in ${CERT_DIR}. Run ./scripts/generate-dev-certs.sh first." >&2
  exit 1
fi

domains=(
  "api.openai.com"
  "api.anthropic.com"
  "generativelanguage.googleapis.com"
)

for domain in "${domains[@]}"; do
  key_file="${CERT_DIR}/${domain}.key"
  csr_file="${CERT_DIR}/${domain}.csr"
  crt_file="${CERT_DIR}/${domain}.crt"
  ext_file="${CERT_DIR}/${domain}.ext"

  echo "Generating certificate for ${domain}"
  openssl genrsa -out "${key_file}" 2048 >/dev/null 2>&1
  openssl req -new -key "${key_file}" -out "${csr_file}" -subj "/CN=${domain}" >/dev/null 2>&1
  cat >"${ext_file}" <<EOF
subjectAltName=DNS:${domain}
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
EOF
  openssl x509 -req \
    -in "${csr_file}" \
    -CA "${CA_CRT}" \
    -CAkey "${CA_KEY}" \
    -CAcreateserial \
    -out "${crt_file}" \
    -days "${DAYS}" \
    -sha256 \
    -extfile "${ext_file}" >/dev/null 2>&1

  rm -f "${csr_file}" "${ext_file}"
done

echo ""
echo "Envoy egress certificates generated in ${CERT_DIR}"
echo "Distribute ${CA_CRT} to workloads that should trust MITM interception."
