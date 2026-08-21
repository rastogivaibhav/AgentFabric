#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${1:-/tmp/a15-native-runtime-evidence}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GDB_COMMIT="fb960c209a888505a64ed17157ca640d732d2640"
HELPER=/tmp/a15_native_runtime_helper
BOOTSTRAP=/tmp/a15_graphene_bootstrap
DB=/tmp/a15_native_runtime.graphenedb
TRACE="$OUT_DIR/native_trace.jsonl"
RUNTIME="$ROOT/pi45_a1/a15_native_runtime"
MODEL_WORLD="$ROOT/pi45_a1/a15_native_model_world"
CORE="$ROOT/pi45_a1/graphenedb_snapshot"

rm -rf "$OUT_DIR" "$DB" "$DB.modelworld"
mkdir -p "$OUT_DIR"
printf '%s\n' "$GDB_COMMIT" > "$OUT_DIR/graphenedb_source_commit.txt"
printf '%s\n' 'vendored_pinned_runtime_closure_no_cross_repo_network_dependency' > "$OUT_DIR/graphenedb_source_mode.txt"

find "$RUNTIME" -type f -print0 | sort -z | xargs -0 sha256sum > "$OUT_DIR/runtime_vendor_SHA256SUMS.txt"
sha256sum \
  "$MODEL_WORLD/src/model_world.cpp" \
  "$MODEL_WORLD/include/graphene/model_world.hpp" \
  "$CORE/src/db.cpp" "$CORE/src/platform_posix.cpp" \
  "$CORE/include/graphene/db.hpp" "$CORE/include/graphene/types.hpp" \
  > "$OUT_DIR/core_SHA256SUMS.txt"

g++ -std=c++20 -O2 -UNDEBUG -I"$CORE/include" \
  "$ROOT/pi45_a1/a15_graphene_bootstrap.cpp" \
  "$CORE/src/db.cpp" "$CORE/src/platform_posix.cpp" \
  -pthread -o "$BOOTSTRAP"
sha256sum "$BOOTSTRAP" "$ROOT/pi45_a1/a15_graphene_bootstrap.cpp" > "$OUT_DIR/bootstrap_SHA256SUMS.txt"
"$BOOTSTRAP" "$DB" | tee "$OUT_DIR/bootstrap.txt"
grep -q 'a15_zero_id_sentinel=PASS id=0' "$OUT_DIR/bootstrap.txt"

g++ -std=c++20 -O2 -UNDEBUG \
  -I"$RUNTIME/include" -I"$MODEL_WORLD/include" -I"$CORE/include" \
  "$ROOT/pi45_a1/a15_native_runtime_helper.cpp" \
  "$CORE/src/db.cpp" "$CORE/src/platform_posix.cpp" \
  "$MODEL_WORLD/src/model_world.cpp" \
  "$RUNTIME/src/epistemic.cpp" \
  "$RUNTIME/src/dialectic.cpp" \
  "$RUNTIME/src/fiber_bundle.cpp" \
  "$RUNTIME/src/stability_critic.cpp" \
  "$RUNTIME/src/epistemic_control.cpp" \
  "$RUNTIME/src/escape.cpp" \
  "$RUNTIME/src/self_healing.cpp" \
  "$RUNTIME/src/path_verifier.cpp" \
  "$RUNTIME/src/hypokosh_runtime.cpp" \
  -pthread -o "$HELPER"
sha256sum "$HELPER" > "$OUT_DIR/helper_SHA256SUMS.txt"
sha256sum "$ROOT/pi45_a1/a15_native_runtime_helper.cpp" > "$OUT_DIR/helper_source_SHA256SUMS.txt"

PYTHONPATH="$ROOT/pi45_a1" python3 "$ROOT/pi45_a1/a15_arc_dialectic_adapter.py" \
  --mode proposal --input "$ROOT/pi45_a1/a15_example_turn.json" \
  --game-id diagnostic --available-actions ACTION1,ACTION2 \
  --native-helper "$HELPER" --db "$DB" --trace "$TRACE" \
  > "$OUT_DIR/proposal_response.json"

PYTHONPATH="$ROOT/pi45_a1" python3 "$ROOT/pi45_a1/a15_arc_dialectic_adapter.py" \
  --mode outcome --input "$ROOT/pi45_a1/a15_example_outcome.json" \
  --game-id diagnostic --known-hypotheses t0-h-interactable,t0-h-inert \
  --native-helper "$HELPER" --db "$DB" --trace "$TRACE" \
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
trace=(out/'native_trace.jsonl').read_text()
assert 'diagnostic' in trace
for response_file in ('proposal_response.json','outcome_response.json'):
    body=(out/response_file).read_text()
    assert 'diagnostic' not in body, body
    assert 'score_delta' not in body and 'level_delta' not in body, body
print('a15_native_runtime_bridge=PASS')
PY

sha256sum "$OUT_DIR/proposal_response.json" "$OUT_DIR/outcome_response.json" "$TRACE" > "$OUT_DIR/evidence_SHA256SUMS.txt"
printf 'a15_native_runtime_bridge=PASS\n'
