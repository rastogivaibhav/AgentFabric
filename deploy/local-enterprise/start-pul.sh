#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo=$(cd "$script_dir/../.." && pwd)
runtime_dir="$repo/.local/enterprise-stack/pul"
package_dir=${PUL_PACKAGE_DIR:?Set PUL_PACKAGE_DIR to the extracted PUL companion directory}

mkdir -p "$runtime_dir"
chmod 700 "$runtime_dir"

if [[ -f "$runtime_dir/pul.pid" ]]; then
  old_pid=$(cat "$runtime_dir/pul.pid" || true)
  if [[ -n "$old_pid" ]] && kill -0 "$old_pid" 2>/dev/null; then
    echo "PUL already running (PID $old_pid)"
    exit 0
  fi
fi

export PUL_STATE="$runtime_dir/pul.db.json"
export PUL_STORE=file
export PUL_SECRETS="$runtime_dir/secrets.json"
export PUL_PACKS_DIR="$package_dir/pul-integrations"

if [[ ! -s "$runtime_dir/admin-token" ]]; then
  python3 -c 'import secrets; print("pul_local_" + secrets.token_urlsafe(32))' > "$runtime_dir/admin-token"
fi
chmod 600 "$runtime_dir/admin-token"
export PUL_ADMIN_TOKEN
PUL_ADMIN_TOKEN=$(cat "$runtime_dir/admin-token")

cd "$package_dir"
./pul init >/dev/null
nohup setsid env PUL_STATE="$PUL_STATE" PUL_STORE="$PUL_STORE" \
  PUL_SECRETS="$PUL_SECRETS" PUL_PACKS_DIR="$PUL_PACKS_DIR" \
  PUL_ADMIN_TOKEN="$PUL_ADMIN_TOKEN" ./pul start 127.0.0.1:8082 \
  >"$runtime_dir/pul.log" 2>&1 < /dev/null &
echo $! > "$runtime_dir/pul.pid"
chmod 600 "$runtime_dir/pul.pid" "$runtime_dir/pul.db.json" 2>/dev/null || true

for _ in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8082/health >/dev/null; then
    echo "PUL healthy (PID $(cat "$runtime_dir/pul.pid"))"
    exit 0
  fi
  sleep 1
done
tail -n 80 "$runtime_dir/pul.log" >&2 || true
exit 1
