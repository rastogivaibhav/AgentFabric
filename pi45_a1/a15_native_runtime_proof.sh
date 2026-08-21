#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${1:-/tmp/a15-native-runtime-evidence}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GDB_COMMIT="fb960c209a888505a64ed17157ca640d732d2640"
GDB_DIR=/tmp/a15-graphenedb-full
HELPER=/tmp/a15_native_runtime_helper
DB=/tmp/a15_native_runtime.graphenedb
TRACE="$OUT_DIR/native_trace.jsonl"

rm -rf "$OUT_DIR" "$GDB_DIR" "$DB" "$DB.modelworld"
mkdir -p "$OUT_DIR"

git clone --quiet https://github.com/rastogivaibhav/graphenedb_v1.git "$GDB_DIR"
git -C "$GDB_DIR" checkout --quiet "$GDB_COMMIT"
test "$(git -C "$GDB_DIR" rev-parse HEAD)" = "$GDB_COMMIT"
printf '%s\n' "$GDB_COMMIT" > "$OUT_DIR/graphenedb_commit.txt"

mapfile -t SOURCES < <(find "$GDB_DIR/src" -maxdepth 1 -type f -name '*.cpp' ! -name 'platform_windows.cpp' | sort)
test "${#SOURCES[@]}" -gt 10

g++ -std=c++20 -O2 -UNDEBUG -I"$GDB_DIR/include" \
  "$ROOT/pi45_a1/a15_native_runtime_helper.cpp" \
  "${SOURCES[@]}" -pthread -o "$HELPER"
sha256sum "$HELPER" > "$OUT_DIR/helper_SHA256SUMS.txt"
sha256sum "$ROOT/pi45_a1/a15_native_runtime_helper.cpp" > "$OUT_DIR/helper_source_SHA256SUMS.txt"

PYTHONPATH="$ROOT/pi45_a1" python3 "$ROOT/pi45_a1/a15_arc_dialectic_adapter.py" \
  --mode proposal \
  --input "$ROOT/pi45_a1/a15_example_turn.json" \
  --game-id diagnostic \
  --available-actions ACTION1,ACTION2 \
  --native-helper "$HELPER" \
  --db "$DB" \
  --trace "$TRACE" \
  > "$OUT_DIR/proposal_response.json"

PYTHONPATH="$ROOT/pi45_a1" python3 "$ROOT/pi45_a1/a15_arc_dialectic_adapter.py" \
  --mode outcome \
  --input "$ROOT/pi45_a1/a15_example_outcome.json" \
  --game-id diagnostic \
  --known-hypotheses h0-interactable,h0-inert \
  --native-helper "$HELPER" \
  --db "$DB" \
  --trace "$TRACE" \
  > "$OUT_DIR/outcome_response.json"

python3 - "$OUT_DIR" <<'PY'
import json, pathlib, sys
out = pathlib.Path(sys.argv[1])
proposal = json.loads((out/'proposal_response.json').read_text())
outcome = json.loads((out/'outcome_response.json').read_text())
for name, obj in [('proposal', proposal), ('outcome', outcome)]:
    receipt = obj['reasoning_receipt']
    required = [
        'graphene_executed','fiber_bundle_built','fiber_bundle_authoritative',
        'stability_critic_executed','epistemic_admissibility_executed',
        'lyapunov_trajectory_executed','escape_considered','convergence_executed',
        'opposition_executed','governed_projection_executed','no_silent_promotion'
    ]
    assert all(receipt.get(k) is True for k in required), (name, receipt)
    assert receipt.get('final_bundle_hash', 0) != 0, (name, receipt)
    assert receipt.get('model_world_event_hash', 0) != 0, (name, receipt)
    assert obj['model_world_nodes'] > 0
    assert obj['model_world_events'] > 0
assert proposal['action_authorized'] is True, proposal
assert proposal['governed_action'] == 'ACTION1', proposal
assert outcome['action_authorized'] is False, outcome
assert outcome['model_world_events'] > proposal['model_world_events'], (proposal, outcome)
assert outcome['reasoning_receipt']['model_world_event_hash'] != proposal['reasoning_receipt']['model_world_event_hash']
print('a15_native_runtime_bridge=PASS')
PY

sha256sum "$OUT_DIR/proposal_response.json" "$OUT_DIR/outcome_response.json" "$TRACE" > "$OUT_DIR/evidence_SHA256SUMS.txt"
printf 'a15_native_runtime_bridge=PASS\n'
