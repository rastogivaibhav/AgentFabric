#!/usr/bin/env bash
set -euo pipefail

GAME="${1:?game required}"
MAX_ACTIONS="${2:-10}"
OUT_DIR="${3:?out dir required}"
ROOT="${GITHUB_WORKSPACE:-$(pwd)}"
MODEL=/tmp/qwen2.5-1.5b-instruct-q4_k_m.gguf
SCRIPT=/mnt/data/pi45a0/arc_a0_baseline.py

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"
test -f "$SCRIPT"
echo '8c251f9bd3241aa1bdaac988ce462b08ab570fb7bcb89ee21c168dc7cf728e62  /mnt/data/pi45a0/arc_a0_baseline.py' | sha256sum -c -
test -f "$MODEL"
cp /tmp/model_SHA256SUMS.txt "$OUT_DIR/model_SHA256SUMS.txt"
git -C /tmp/llama.cpp rev-parse HEAD > "$OUT_DIR/llama_cpp_commit.txt"
sha256sum "$SCRIPT" > "$OUT_DIR/a0_script_SHA256SUMS.txt"
printf '%s\n' \
  "variant=a0-control" \
  "game=$GAME" \
  "max_actions=$MAX_ACTIONS" \
  "memory_window=6" \
  "context_size=4096" \
  "graphenedb=false" \
  "persistent_model_world=false" \
  "frozen_historical_control=true" \
  > "$OUT_DIR/paper_treatment_contract.txt"

/tmp/llama.cpp/build/bin/llama-server \
  -m "$MODEL" -c 4096 --parallel 1 \
  --host 127.0.0.1 --port 9090 --alias pi45a0-qwen25-15b \
  > "$OUT_DIR/llama_server.log" 2>&1 &
PID=$!
trap 'kill "$PID" 2>/dev/null || true' EXIT
ready=0
for _ in $(seq 1 120); do
  if curl -fsS http://127.0.0.1:9090/health >/dev/null 2>&1; then ready=1; break; fi
  sleep 1
done
test "$ready" = 1

set +e
MODEL_ALIAS=pi45a0-qwen25-15b /tmp/pi45venv/bin/python "$SCRIPT" \
  --games "$GAME" --max-actions "$MAX_ACTIONS" --memory-window 6 --out "$OUT_DIR" \
  2>&1 | tee "$OUT_DIR/runner_stdout.log"
RC=${PIPESTATUS[0]}
set -e
printf '%s\n' "$RC" > "$OUT_DIR/runner_exit_code.txt"
test "$RC" -eq 0
test -f "$OUT_DIR/summary.json"

OUT_DIR="$OUT_DIR" python3 - <<'PY'
import json, os
from pathlib import Path
out=Path(os.environ['OUT_DIR'])
s=json.loads((out/'summary.json').read_text())
assert s['graphenedb_enabled'] is False
assert s['persistent_world_model'] is False
assert s['model_sha256']=='6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e'
json.dump({'valid_measurement':True,'invalid_reasons':[]},open(out/'measurement_validity.json','w'),indent=2,sort_keys=True)
print('a0_arc_paper_measurement=VALID')
PY
find "$OUT_DIR" -type f ! -name SHA256SUMS.txt -print0 | sort -z | xargs -0 -r sha256sum > "$OUT_DIR/SHA256SUMS.txt"
echo 'a0_run_game_paper=PASS'
