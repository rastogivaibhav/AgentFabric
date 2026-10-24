#!/usr/bin/env bash
set -euo pipefail

runtime=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
pid_file="$runtime/state/service.pid"

if [[ ! -f "$pid_file" ]]; then
  echo "Enterprise agent service is not running"
  exit 0
fi

pid=$(cat "$pid_file")
if kill -0 "$pid" 2>/dev/null; then
  kill "$pid"
  for _ in $(seq 1 20); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.25
  done
fi
rm -f "$pid_file"
echo "Enterprise agent service stopped"
