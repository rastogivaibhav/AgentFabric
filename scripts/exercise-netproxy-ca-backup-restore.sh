#!/usr/bin/env bash
set -euo pipefail

NETPROXY_CA_CERT_FILE="${NETPROXY_CA_CERT_FILE:-}"
NETPROXY_CA_KEY_FILE="${NETPROXY_CA_KEY_FILE:-}"
OUTPUT_DIR="${OUTPUT_DIR:-./artifacts/netproxy-ca-drill}"
OUTPUT_PATH="${OUTPUT_PATH:-}"
JSON_OUTPUT_PATH="${JSON_OUTPUT_PATH:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

[[ -n "${OUTPUT_PATH}" ]] || OUTPUT_PATH="${REPO_ROOT}/netproxy-ca-drill.md"

if [[ -z "${NETPROXY_CA_CERT_FILE}" || -z "${NETPROXY_CA_KEY_FILE}" ]]; then
  echo "NETPROXY_CA_CERT_FILE and NETPROXY_CA_KEY_FILE are required" >&2
  exit 1
fi

if [[ ! -f "${NETPROXY_CA_CERT_FILE}" ]]; then
  echo "NetProxy CA cert file not found: ${NETPROXY_CA_CERT_FILE}" >&2
  exit 1
fi

if [[ ! -f "${NETPROXY_CA_KEY_FILE}" ]]; then
  echo "NetProxy CA key file not found: ${NETPROXY_CA_KEY_FILE}" >&2
  exit 1
fi

hash_file() {
  local path="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${path}" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "${path}" | awk '{print $1}'
  else
    openssl dgst -sha256 "${path}" | awk '{print $NF}'
  fi
}

mkdir -p "${OUTPUT_DIR}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_dir="${OUTPUT_DIR}/backup-${stamp}"
restore_dir="${OUTPUT_DIR}/restore-${stamp}"
mkdir -p "${backup_dir}" "${restore_dir}"

backup_cert="${backup_dir}/govagn-netproxy-ca.crt"
backup_key="${backup_dir}/govagn-netproxy-ca.key"
restore_cert="${restore_dir}/govagn-netproxy-ca.crt"
restore_key="${restore_dir}/govagn-netproxy-ca.key"

cp "${NETPROXY_CA_CERT_FILE}" "${backup_cert}"
cp "${NETPROXY_CA_KEY_FILE}" "${backup_key}"
cp "${backup_cert}" "${restore_cert}"
cp "${backup_key}" "${restore_key}"

source_cert_hash="$(hash_file "${NETPROXY_CA_CERT_FILE}")"
source_key_hash="$(hash_file "${NETPROXY_CA_KEY_FILE}")"
restore_cert_hash="$(hash_file "${restore_cert}")"
restore_key_hash="$(hash_file "${restore_key}")"

[[ "${source_cert_hash}" == "${restore_cert_hash}" ]] || { echo "restored cert hash mismatch" >&2; exit 1; }
[[ "${source_key_hash}" == "${restore_key_hash}" ]] || { echo "restored key hash mismatch" >&2; exit 1; }

cert_subject="$(openssl x509 -in "${restore_cert}" -noout -subject | sed 's/^subject=//')"
cert_issuer="$(openssl x509 -in "${restore_cert}" -noout -issuer | sed 's/^issuer=//')"
cert_fingerprint="$(openssl x509 -in "${restore_cert}" -noout -fingerprint -sha256 | sed 's/^sha256 Fingerprint=//')"
cert_not_after="$(openssl x509 -in "${restore_cert}" -noout -enddate | sed 's/^notAfter=//')"
cert_pubkey="$(openssl x509 -in "${restore_cert}" -pubkey -noout)"
key_pubkey="$(openssl pkey -in "${restore_key}" -pubout)"

if [[ "${cert_pubkey}" != "${key_pubkey}" ]]; then
  echo "restored key does not match restored certificate" >&2
  exit 1
fi

summary="# Govagn NetProxy CA Backup and Restore Drill

- Validation result: PASS
- Generated at: $(date -u '+%Y-%m-%d %H:%M:%S UTC')
- Source cert: \`${NETPROXY_CA_CERT_FILE}\`
- Source key: \`${NETPROXY_CA_KEY_FILE}\`
- Backup artifact directory: \`${backup_dir}\`
- Restore verification directory: \`${restore_dir}\`

## Certificate Identity

- Subject: \`${cert_subject}\`
- Issuer: \`${cert_issuer}\`
- SHA-256 fingerprint: \`${cert_fingerprint}\`
- Not after: \`${cert_not_after}\`

## Verification

- Source cert hash: \`${source_cert_hash}\`
- Restored cert hash: \`${restore_cert_hash}\`
- Source key hash: \`${source_key_hash}\`
- Restored key hash: \`${restore_key_hash}\`
- Cert/key public key match: \`verified\`

## Operator Notes

- This drill copies the persisted NetProxy CA files into a timestamped backup directory.
- It restores those copies into an isolated verification directory without mutating the live source files.
- Use this artifact as release evidence that backup and restore steps were exercised in the current cycle.
"

printf '%s\n' "${summary}" >"${OUTPUT_PATH}"

if [[ -n "${JSON_OUTPUT_PATH}" ]]; then
  cat >"${JSON_OUTPUT_PATH}" <<EOF
{"validation_result":"PASS","generated_at":"$(date -u '+%Y-%m-%dT%H:%M:%SZ')","source_cert":"${NETPROXY_CA_CERT_FILE}","source_key":"${NETPROXY_CA_KEY_FILE}","backup_dir":"${backup_dir}","restore_dir":"${restore_dir}","source_cert_hash":"${source_cert_hash}","restored_cert_hash":"${restore_cert_hash}","source_key_hash":"${source_key_hash}","restored_key_hash":"${restore_key_hash}","cert_subject":"${cert_subject}","cert_issuer":"${cert_issuer}","cert_fingerprint":"${cert_fingerprint}","cert_not_after":"${cert_not_after}"}
EOF
fi

echo "NetProxy CA drill summary written to ${OUTPUT_PATH}"
