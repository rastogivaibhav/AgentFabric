#!/usr/bin/env bash
set -euo pipefail

GAME="${1:?game required}"
MAX_ACTIONS="${2:-10}"
OUT_DIR="${3:?out dir required}"
ROOT="$GITHUB_WORKSPACE"
RUNTIME="$ROOT/pi45_a1/a15_native_runtime"
MODEL_WORLD="$ROOT/pi45_a1/a15_native_model_world"
CORE="$ROOT/pi45_a1/graphenedb_snapshot"
HELPER=/tmp/a15_native_runtime_helper
BOOTSTRAP=/tmp/a15_graphene_bootstrap
DUMPER=/tmp/a15_model_world_dump
MODEL=/tmp/qwen2.5-1.5b-instruct-q4_k_m.gguf

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

python3 -m py_compile \
  "$ROOT/pi45_a1/a15_arc_diagnostic_runner.py" \
  "$ROOT/pi45_a1/a15_arc_dialectic_adapter.py" \
  "$ROOT/pi45_a1/a15_proposal_prompt.py" \
  "$ROOT/pi45_a1/a15_outcome_prompt.py" \
  "$ROOT/pi45_a1/a15_contract_gate.py"
PYTHONPATH="$ROOT/pi45_a1" python3 -m unittest -v \
  pi45_a1/test_a15_arc_dialectic_adapter.py \
  pi45_a1/test_a15_proposal_prompt.py \
  pi45_a1/test_a15_outcome_prompt.py \
  > "$OUT_DIR/unit_tests.log" 2>&1

# Build the DB-only id-0 reservation helper.
g++ -std=c++20 -O2 -UNDEBUG -I"$CORE/include" \
  "$ROOT/pi45_a1/a15_graphene_bootstrap.cpp" \
  "$CORE/src/db.cpp" "$CORE/src/platform_posix.cpp" \
  -pthread -o "$BOOTSTRAP"

# Build the full native dialectical bridge from the vendored pinned closure.
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

# Build a read-only sanitized ModelWorld projection for next-turn feedback.
# ModelWorld links the platform durable-replace helper even for a read-only dump.
g++ -std=c++20 -O2 -UNDEBUG \
  -I"$RUNTIME/include" -I"$MODEL_WORLD/include" -I"$CORE/include" \
  "$ROOT/pi45_a1/a15_model_world_dump.cpp" \
  "$MODEL_WORLD/src/model_world.cpp" \
  "$CORE/src/platform_posix.cpp" \
  -pthread -o "$DUMPER"

sha256sum "$HELPER" "$BOOTSTRAP" "$DUMPER" > "$OUT_DIR/native_binaries_SHA256SUMS.txt"
sha256sum \
  "$ROOT/pi45_a1/a15_arc_diagnostic_runner.py" \
  "$ROOT/pi45_a1/a15_arc_dialectic_adapter.py" \
  "$ROOT/pi45_a1/a15_proposal_prompt.py" \
  "$ROOT/pi45_a1/a15_outcome_prompt.py" \
  "$ROOT/pi45_a1/a15_state_contract.json" \
  "$ROOT/pi45_a1/a15_reasoning_constitution.json" \
  > "$OUT_DIR/a15_sources_SHA256SUMS.txt"
cp /tmp/model_SHA256SUMS.txt "$OUT_DIR/model_SHA256SUMS.txt"
git -C /tmp/llama.cpp rev-parse HEAD > "$OUT_DIR/llama_cpp_commit.txt"
printf '%s\n' \
  "game=$GAME" \
  "max_actions=$MAX_ACTIONS" \
  "model=qwen2.5-1.5b-instruct-q4_k_m" \
  "temperature=0" \
  "seed=4242" \
  "perception=grid_plus_opaque_actions_only" \
  "evaluator_metadata_in_reasoning=false" \
  "native_runtime=CompleteHypoKoshRuntime" \
  "persistent_model_world=true" \
  > "$OUT_DIR/treatment_contract.txt"

# Dedicated local inference server. One request at a time to preserve determinism.
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

PYTHONPATH="$ROOT/pi45_a1" /tmp/pi45venv/bin/python "$ROOT/pi45_a1/a15_arc_diagnostic_runner.py" \
  --games "$GAME" --max-actions "$MAX_ACTIONS" \
  --endpoint http://127.0.0.1:9090/v1/chat/completions \
  --out "$OUT_DIR" \
  --native-helper "$HELPER" \
  --bootstrap "$BOOTSTRAP" \
  --modelworld-dump "$DUMPER" \
  --contract-path "$ROOT/pi45_a1/a15_state_contract.json" \
  2>&1 | tee "$OUT_DIR/runner_stdout.log"

python3 - "$OUT_DIR" "$MAX_ACTIONS" <<'PY'
import json, pathlib, sys
out=pathlib.Path(sys.argv[1]); max_actions=int(sys.argv[2])
s=json.loads((out/'summary.json').read_text())
assert s['evaluator_metadata_in_reasoning'] is False
assert s['native_dialectical_runtime'] is True
assert s['persistent_model_world'] is True
assert len(s['games']) == 1
g=s['games'][0]
inst=g.get('instrumentation') or {}
for k in ['proposal_errors','outcome_errors','native_errors','governance_denials','step_errors','null_steps','empty_action_space']:
    assert int(inst.get(k,0)) == 0, (k, inst)
actions=int(g.get('actions',0))
terminal=str(g.get('terminal_state',''))
assert actions == max_actions or terminal == 'WIN', (actions, max_actions, terminal)
assert int(g.get('distinct_hypotheses',0)) >= 2, g
assert int(g.get('distinct_goals',0)) >= 1, g
assert int(g.get('model_world_nodes',0)) > 0, g
assert int(g.get('model_world_event_hash',0)) != 0, g
assert int(inst.get('outcomes_recorded',0)) == actions, (inst, actions)
assert int(inst.get('lyapunov_checks',0)) == actions, (inst, actions)

# Model-facing calls must not contain evaluator-only keys/identifiers.
for p in out.glob('*.a15.model_calls.jsonl'):
    text=p.read_text().lower()
    for forbidden in ['game_id','win_levels','levels_completed','score_delta','level_delta','scorecard','official_success']:
        assert forbidden not in text, (forbidden, p)

# Native ModelWorld trace is allowed evaluator metadata only under the dedicated evaluator object.
for p in out.glob('*.a15.turns.jsonl'):
    for line in p.read_text().splitlines():
        row=json.loads(line)
        epistemic={k:v for k,v in row.items() if k != 'evaluator'}
        encoded=json.dumps(epistemic, sort_keys=True).lower()
        for forbidden in ['win_levels','levels_completed','score_delta','level_delta','scorecard','official_success']:
            assert forbidden not in encoded, (forbidden, row.get('turn'))

records=[json.loads(x) for x in (out/'transport_proxy.jsonl').read_text().splitlines() if x.strip()]
posts=[r for r in records if r.get('method')=='POST']
assert len(posts) == 2*actions, (len(posts), actions)
assert not any(r.get('client_disconnected') for r in posts), posts
assert not any(r.get('error') or (r.get('status') or 0)>=500 for r in posts), posts
assert max((int(r.get('max_active_observed',0)) for r in posts), default=0) <= 1
transport={
 'requests':len(posts),
 'truncated_requests':sum(bool(r.get('truncated')) for r in posts),
 'max_concurrent_inference_requests':max((int(r.get('max_active_observed',0)) for r in posts),default=0),
 'transport_errors':sum(bool(r.get('error')) or (r.get('status') or 0)>=500 for r in posts),
 'client_disconnects':sum(bool(r.get('client_disconnected')) for r in posts),
 'max_upstream_s':max((float(r.get('upstream_elapsed_s',0)) for r in posts),default=0),
}
json.dump(transport, open(out/'transport_metrics.json','w'), indent=2, sort_keys=True)
print('a15_diagnostic_gate=PASS')
print(json.dumps({'game':g,'transport':transport},indent=2,sort_keys=True))
PY

find "$OUT_DIR" -type f ! -name SHA256SUMS.txt -print0 | sort -z | xargs -0 -r sha256sum > "$OUT_DIR/SHA256SUMS.txt"
echo 'a15_run_diagnostic=PASS'
