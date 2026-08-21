#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${1:-results/pi45a15-synthetic-harness}"
ROOT="${GITHUB_WORKSPACE:-$(pwd)}"
MODEL_WORLD="$ROOT/pi45_a1/a15_native_model_world"
RUNTIME="$ROOT/pi45_a1/a15_native_runtime"
CORE="$ROOT/pi45_a1/graphenedb_snapshot"
HELPER=/tmp/a15_native_runtime_helper
BOOTSTRAP=/tmp/a15_graphene_bootstrap
DUMPER=/tmp/a15_model_world_dump
MODEL=/tmp/qwen2.5-1.5b-instruct-q4_k_m.gguf

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

# The synthetic H1 runner must have no live target-game imports or execution surface.
! grep -Eq '(^|[[:space:]])(import|from)[[:space:]]+(arc_agi|arcengine)' "$ROOT/pi45_a1/a15_synthetic_world_harness.py"
! grep -q 'arc\.make\|get_environments\|env\.step\|Arcade(' "$ROOT/pi45_a1/a15_synthetic_world_harness.py"
printf '%s\n' 'arc_static_execution_surface=ABSENT' > "$OUT_DIR/arc_execution_guard.txt"

# H0: contract/prompt/adapter tests first. No environment is created here.
python3 -m py_compile \
  "$ROOT/pi45_a1/a15_contract_gate.py" \
  "$ROOT/pi45_a1/a15_arc_dialectic_adapter.py" \
  "$ROOT/pi45_a1/a15_proposal_prompt.py" \
  "$ROOT/pi45_a1/a15_outcome_prompt.py" \
  "$ROOT/pi45_a1/a15_synthetic_world_harness.py"
PYTHONPATH="$ROOT/pi45_a1" python3 -m unittest -v \
  pi45_a1/test_a15_arc_dialectic_adapter.py \
  pi45_a1/test_a15_proposal_prompt.py \
  pi45_a1/test_a15_outcome_prompt.py \
  > "$OUT_DIR/h0_unit_tests.log" 2>&1
PYTHONPATH="$ROOT/pi45_a1" python3 "$ROOT/pi45_a1/a15_contract_gate.py" \
  > "$OUT_DIR/h0_contract_gate.log" 2>&1

# H0: execute the real native CompleteHypoKoshRuntime proof.
bash "$ROOT/pi45_a1/a15_native_runtime_proof.sh" "$OUT_DIR/h0_native_runtime" \
  > "$OUT_DIR/h0_native_runtime_stdout.log" 2>&1
grep -q 'a15_native_runtime_bridge=PASS' "$OUT_DIR/h0_native_runtime_stdout.log"

# Reuse the pinned Qwen/llama.cpp preparation harness. It may install the ARC
# package for parity, but this synthetic path never creates or steps a game.
bash "$ROOT/pi45_a1/a13_prepare.sh" > "$OUT_DIR/model_prepare.log" 2>&1

test -x "$HELPER"
test -x "$BOOTSTRAP"
test -f "$MODEL"

# ModelWorld uses durable_replace while saving; link the platform closure even
# though this helper only reads the resulting snapshot.
g++ -std=c++20 -O2 -UNDEBUG \
  -I"$RUNTIME/include" -I"$MODEL_WORLD/include" -I"$CORE/include" \
  "$ROOT/pi45_a1/a15_model_world_dump.cpp" \
  "$MODEL_WORLD/src/model_world.cpp" \
  "$CORE/src/platform_posix.cpp" \
  -pthread -o "$DUMPER"

sha256sum "$HELPER" "$BOOTSTRAP" "$DUMPER" > "$OUT_DIR/native_binaries_SHA256SUMS.txt"
sha256sum \
  "$ROOT/pi45_a1/a15_synthetic_world_harness.py" \
  "$ROOT/pi45_a1/a15_arc_dialectic_adapter.py" \
  "$ROOT/pi45_a1/a15_proposal_prompt.py" \
  "$ROOT/pi45_a1/a15_outcome_prompt.py" \
  "$ROOT/pi45_a1/a15_contract_gate.py" \
  "$ROOT/pi45_a1/a15_state_contract.json" \
  "$ROOT/pi45_a1/a15_reasoning_constitution.json" \
  > "$OUT_DIR/a15_harness_sources_SHA256SUMS.txt"
cp /tmp/model_SHA256SUMS.txt "$OUT_DIR/model_SHA256SUMS.txt"
git -C /tmp/llama.cpp rev-parse HEAD > "$OUT_DIR/llama_cpp_commit.txt"

printf '%s\n' \
  'harness=synthetic_hidden_interactive_world' \
  'arc_game_execution=false' \
  'model=qwen2.5-1.5b-instruct-q4_k_m' \
  'temperature=0' \
  'seed=4242' \
  'min_turns=4' \
  'max_turns=8' \
  'perception=integer_grid_plus_opaque_actions_only' \
  'native_runtime=CompleteHypoKoshRuntime' \
  'persistent_model_world=true' \
  'evaluator_metadata_in_reasoning=false' \
  'outcome_repair=max_one_same-model-validation-retry' \
  > "$OUT_DIR/harness_contract.txt"

/tmp/llama.cpp/build/bin/llama-server \
  -m "$MODEL" -c 8192 --parallel 1 \
  --host 127.0.0.1 --port 9091 --alias pi45a15-qwen25-15b \
  > "$OUT_DIR/llama_server.log" 2>&1 &
SERVER_PID=$!
python3 "$ROOT/pi45_a1/transport_safe_chat_proxy.py" \
  --listen 9090 --target http://127.0.0.1:9091 \
  --budget-chars 12000 --upstream-timeout 360 \
  --log "$OUT_DIR/transport_proxy.jsonl" \
  > "$OUT_DIR/transport_proxy.log" 2>&1 &
PROXY_PID=$!
trap 'kill "$PROXY_PID" "$SERVER_PID" 2>/dev/null || true' EXIT

ready=0
for _ in $(seq 1 120); do
  if curl -fsS http://127.0.0.1:9090/health >/dev/null 2>&1; then ready=1; break; fi
  sleep 1
done
test "$ready" = 1

PYTHONPATH="$ROOT/pi45_a1" /tmp/pi45venv/bin/python "$ROOT/pi45_a1/a15_synthetic_world_harness.py" \
  --endpoint http://127.0.0.1:9090/v1/chat/completions \
  --out "$OUT_DIR/h1" \
  --native-helper "$HELPER" \
  --bootstrap "$BOOTSTRAP" \
  --modelworld-dump "$DUMPER" \
  --contract-path "$ROOT/pi45_a1/a15_state_contract.json" \
  --min-turns 4 --max-turns 8 \
  2>&1 | tee "$OUT_DIR/h1_stdout.log"

grep -q 'a15_synthetic_model_in_loop_harness=PASS' "$OUT_DIR/h1_stdout.log"

python3 - "$OUT_DIR" <<'PY'
import json, pathlib, sys
out=pathlib.Path(sys.argv[1])
summary=json.loads((out/'h1/summary.json').read_text())
inst=summary.get('instrumentation') or {}
assert summary['arc_environment_used'] is False
assert summary['evaluator_metadata_in_reasoning'] is False
assert summary['native_dialectical_runtime'] is True
assert summary['persistent_model_world'] is True
assert summary['turns_executed'] >= summary['min_turns']
assert summary['unique_actions'] >= 2
assert summary['meaningful_changes'] >= 1
assert summary['max_same_state_action_trials'] <= 2
assert summary['model_world_nodes'] > 0
assert summary['model_world_event_hash'] != 0
repairs=int(inst.get('outcome_repairs_attempted',0))
assert repairs <= int(summary['turns_executed'])
assert int(inst.get('outcome_repairs_succeeded',0)) == repairs

# Hidden evaluator rules are kept in a separate artifact. They must never be
# copied into model-call or epistemic-turn records.
model_text=(out/'h1/synthetic.a15.model_calls.jsonl').read_text().lower()
turn_rows=[json.loads(line) for line in (out/'h1/synthetic.a15.turns.jsonl').read_text().splitlines() if line.strip()]
for forbidden in ['ft09','bp35','ls20','win_levels','levels_completed','score_delta','level_delta','scorecard','official_success']:
    assert forbidden not in model_text, forbidden
    for row in turn_rows:
        epistemic={k:v for k,v in row.items() if k != 'evaluator'}
        assert forbidden not in json.dumps(epistemic,sort_keys=True).lower(), (forbidden,row.get('turn'))

records=[json.loads(line) for line in (out/'transport_proxy.jsonl').read_text().splitlines() if line.strip()]
posts=[r for r in records if r.get('method')=='POST']
turns=int(summary['turns_executed'])
expected=2*turns+repairs
assert len(posts) == expected, (len(posts),expected,turns,repairs)
assert not any(r.get('client_disconnected') for r in posts)
assert not any(r.get('error') or (r.get('status') or 0) >= 500 for r in posts)
assert max((int(r.get('max_active_observed',0)) for r in posts),default=0) <= 1
transport={
  'requests':len(posts),
  'expected_requests':expected,
  'outcome_repair_requests':repairs,
  'truncated_requests':sum(bool(r.get('truncated')) for r in posts),
  'transport_errors':sum(bool(r.get('error')) or (r.get('status') or 0)>=500 for r in posts),
  'client_disconnects':sum(bool(r.get('client_disconnected')) for r in posts),
  'max_concurrent_inference_requests':max((int(r.get('max_active_observed',0)) for r in posts),default=0),
  'max_upstream_s':max((float(r.get('upstream_elapsed_s',0)) for r in posts),default=0),
}
json.dump(transport,open(out/'transport_metrics.json','w'),indent=2,sort_keys=True)
print('a15_synthetic_harness_post_gate=PASS')
print(json.dumps({'summary':summary,'transport':transport},indent=2,sort_keys=True))
PY

find "$OUT_DIR" -type f ! -name SHA256SUMS.txt -print0 | sort -z | xargs -0 -r sha256sum > "$OUT_DIR/SHA256SUMS.txt"
echo 'a15_run_synthetic_harness=PASS'
