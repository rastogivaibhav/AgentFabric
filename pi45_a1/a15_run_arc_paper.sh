#!/usr/bin/env bash
set -euo pipefail

GAME="${1:?game required}"
MAX_ACTIONS="${2:-10}"
OUT_DIR="${3:?out dir required}"
ROOT="${GITHUB_WORKSPACE:-$(pwd)}"
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
  "$ROOT/pi45_a1/a15_arc_paper_runner.py" \
  "$ROOT/pi45_a1/a15_arc_dialectic_adapter.py" \
  "$ROOT/pi45_a1/a15_proposal_prompt.py" \
  "$ROOT/pi45_a1/a15_outcome_prompt.py" \
  "$ROOT/pi45_a1/a15_contract_gate.py"
PYTHONPATH="$ROOT/pi45_a1" python3 -m unittest -v \
  pi45_a1/test_a15_arc_dialectic_adapter.py \
  pi45_a1/test_a15_proposal_prompt.py \
  pi45_a1/test_a15_outcome_prompt.py \
  > "$OUT_DIR/unit_tests.log" 2>&1
PYTHONPATH="$ROOT/pi45_a1" python3 "$ROOT/pi45_a1/a15_contract_gate.py" \
  > "$OUT_DIR/contract_gate.log" 2>&1

# Native GrapheneDB / ModelWorld closure. No Python reasoning fallback.
g++ -std=c++20 -O2 -UNDEBUG -I"$CORE/include" \
  "$ROOT/pi45_a1/a15_graphene_bootstrap.cpp" \
  "$CORE/src/db.cpp" "$CORE/src/platform_posix.cpp" \
  -pthread -o "$BOOTSTRAP"

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

g++ -std=c++20 -O2 -UNDEBUG \
  -I"$RUNTIME/include" -I"$MODEL_WORLD/include" -I"$CORE/include" \
  "$ROOT/pi45_a1/a15_model_world_dump.cpp" \
  "$MODEL_WORLD/src/model_world.cpp" "$CORE/src/platform_posix.cpp" \
  -pthread -o "$DUMPER"

sha256sum "$HELPER" "$BOOTSTRAP" "$DUMPER" > "$OUT_DIR/native_binaries_SHA256SUMS.txt"
sha256sum \
  "$ROOT/pi45_a1/a15_arc_diagnostic_runner.py" \
  "$ROOT/pi45_a1/a15_arc_paper_runner.py" \
  "$ROOT/pi45_a1/a15_arc_dialectic_adapter.py" \
  "$ROOT/pi45_a1/a15_proposal_prompt.py" \
  "$ROOT/pi45_a1/a15_outcome_prompt.py" \
  "$ROOT/pi45_a1/a15_contract_gate.py" \
  "$ROOT/pi45_a1/a15_state_contract.json" \
  "$ROOT/pi45_a1/a15_reasoning_constitution.json" \
  > "$OUT_DIR/a15_paper_sources_SHA256SUMS.txt"
cp /tmp/model_SHA256SUMS.txt "$OUT_DIR/model_SHA256SUMS.txt"
git -C /tmp/llama.cpp rev-parse HEAD > "$OUT_DIR/llama_cpp_commit.txt"
printf '%s\n' \
  "variant=a15-dmw" \
  "game=$GAME" \
  "max_actions=$MAX_ACTIONS" \
  "model=qwen2.5-1.5b-instruct-q4_k_m" \
  "temperature=0" \
  "seed=4242" \
  "context_size=8192" \
  "perception=structured-grid-plus-opaque-actions" \
  "observation_authority=deterministic-runtime" \
  "proposal_language=compact-semantics-over-runtime-evidence-catalog" \
  "dialectic_control_owner=CompleteHypoKoshRuntime" \
  "persistent_model_world=true" \
  "proposal_failure_is_performance_result=true" \
  "outcome_failure_is_performance_result=true" \
  "validity_blockers=transport_native_environment_evaluator-leakage" \
  > "$OUT_DIR/paper_treatment_contract.txt"

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

set +e
PYTHONPATH="$ROOT/pi45_a1" /tmp/pi45venv/bin/python "$ROOT/pi45_a1/a15_arc_paper_runner.py" \
  --games "$GAME" --max-actions "$MAX_ACTIONS" \
  --endpoint http://127.0.0.1:9090/v1/chat/completions \
  --out "$OUT_DIR" \
  --native-helper "$HELPER" --bootstrap "$BOOTSTRAP" --modelworld-dump "$DUMPER" \
  --contract-path "$ROOT/pi45_a1/a15_state_contract.json" \
  2>&1 | tee "$OUT_DIR/runner_stdout.log"
RUNNER_RC=${PIPESTATUS[0]}
set -e
printf '%s\n' "$RUNNER_RC" > "$OUT_DIR/runner_exit_code.txt"
# A process crash is an invalid measurement. Model contract rejection is handled
# inside the runner and still returns a summary with rc=0.
test "$RUNNER_RC" -eq 0
test -f "$OUT_DIR/summary.json"

OUT_DIR="$OUT_DIR" MAX_ACTIONS="$MAX_ACTIONS" python3 - <<'PY'
import json, os
from pathlib import Path

out=Path(os.environ['OUT_DIR'])
s=json.loads((out/'summary.json').read_text())
assert s['model_sha256']=='6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e'
assert s['evaluator_metadata_in_reasoning'] is False
assert s['native_dialectical_runtime'] is True
assert s['persistent_model_world'] is True
assert len(s.get('games') or []) == 1
g=s['games'][0]
inst=g.get('instrumentation') or {}

# These invalidate the experiment itself. Proposal/outcome/governance failures
# are deliberately NOT here: they are treatment performance outcomes.
invalid_reasons=[]
if g.get('error'):
    invalid_reasons.append('environment_setup:'+str(g.get('error')))
for key in ['native_errors','step_errors','null_steps','empty_action_space']:
    if int(inst.get(key,0)):
        invalid_reasons.append(f'{key}={inst.get(key)}')

# No evaluator-only state may enter epistemic turn records.
for path in out.glob('*.a15.turns.jsonl'):
    for line in path.read_text().splitlines():
        if not line.strip():
            continue
        row=json.loads(line)
        epistemic={k:v for k,v in row.items() if k!='evaluator'}
        encoded=json.dumps(epistemic,sort_keys=True).lower()
        for forbidden in ['game_id','win_levels','levels_completed','score_delta','level_delta','scorecard','official_success','level_scores']:
            if forbidden in encoded:
                invalid_reasons.append(f'evaluator_leak:{forbidden}:turn={row.get("turn")}')

records=[json.loads(x) for x in (out/'transport_proxy.jsonl').read_text().splitlines() if x.strip()]
posts=[r for r in records if r.get('method')=='POST']
transport={
 'requests':len(posts),
 'truncated_requests':sum(bool(r.get('truncated')) for r in posts),
 'max_concurrent_inference_requests':max((int(r.get('max_active_observed',0)) for r in posts),default=0),
 'transport_errors':sum(bool(r.get('error')) or (r.get('status') or 0)>=500 for r in posts),
 'client_disconnects':sum(bool(r.get('client_disconnected')) for r in posts),
 'max_upstream_s':max((float(r.get('upstream_elapsed_s',0)) for r in posts),default=0),
}
if transport['transport_errors']:
    invalid_reasons.append(f'transport_errors={transport["transport_errors"]}')
if transport['client_disconnects']:
    invalid_reasons.append(f'client_disconnects={transport["client_disconnects"]}')
if transport['max_concurrent_inference_requests'] > 1:
    invalid_reasons.append('inference_concurrency_gt_1')
json.dump(transport,open(out/'transport_metrics.json','w'),indent=2,sort_keys=True)

wrapper=s.get('paper_wrapper') or {}
metrics={
 'variant':'a15-dmw',
 'valid_measurement':not invalid_reasons,
 'invalid_reasons':invalid_reasons,
 'game_id':g.get('game_id'),
 'actions':int(g.get('actions',0)),
 'max_actions':int(os.environ['MAX_ACTIONS']),
 'levels_gained':int(g.get('levels_gained',0)),
 'levels_completed':int(g.get('levels_completed',0)),
 'terminal_state':g.get('terminal_state'),
 'score':(s.get('scorecard') or {}).get('score') if isinstance(s.get('scorecard'),dict) else None,
 'action_counts':g.get('action_counts') or {},
 'unique_actions':int(g.get('unique_actions',0)),
 'no_ops':int(inst.get('no_ops',0)),
 'repeated_state_action':int(inst.get('repeated_state_action',0)),
 'first_meaningful_change_turn':g.get('first_meaningful_change_turn'),
 'proposal_errors':int(inst.get('proposal_errors',0)),
 'outcome_errors':int(inst.get('outcome_errors',0)),
 'governance_denials':int(inst.get('governance_denials',0)),
 'hypotheses_generated':int(inst.get('hypotheses_generated',0)),
 'candidate_goals_generated':int(inst.get('candidate_goals_generated',0)),
 'hypotheses_supported':int(inst.get('hypotheses_supported',0)),
 'hypotheses_contradicted':int(inst.get('hypotheses_contradicted',0)),
 'lyapunov_checks':int(inst.get('lyapunov_checks',0)),
 'escape_considered':int(inst.get('escape_considered',0)),
 'distinct_hypotheses':int(g.get('distinct_hypotheses',0)),
 'distinct_goals':int(g.get('distinct_goals',0)),
 'model_world_nodes':int(g.get('model_world_nodes',0)),
 'model_world_event_hash':int(g.get('model_world_event_hash',0)),
 'proposal_repairs_attempted':int(wrapper.get('proposal_repairs_attempted',0)),
 'proposal_repairs_succeeded':int(wrapper.get('proposal_repairs_succeeded',0)),
 'outcome_repairs_attempted':int(wrapper.get('outcome_repairs_attempted',0)),
 'outcome_repairs_succeeded':int(wrapper.get('outcome_repairs_succeeded',0)),
 'native_opposition_rounds':int(wrapper.get('native_opposition_rounds',0)),
 'native_reopen_events':int(wrapper.get('native_reopen_events',0)),
 'alternatives_reopened':int(wrapper.get('alternatives_reopened',0)),
 'native_primary_hypothesis_selections':int(wrapper.get('native_primary_hypothesis_selections',0)),
 'model_sha256':s.get('model_sha256'),
}
json.dump(metrics,open(out/'paper_metrics.json','w'),indent=2,sort_keys=True)
json.dump({'valid_measurement':not invalid_reasons,'invalid_reasons':invalid_reasons},open(out/'measurement_validity.json','w'),indent=2,sort_keys=True)
print('a15_arc_paper_measurement=' + ('VALID' if not invalid_reasons else 'INVALID'))
print(json.dumps(metrics,indent=2,sort_keys=True))
if invalid_reasons:
    raise SystemExit(3)
PY

find "$OUT_DIR" -type f ! -name SHA256SUMS.txt -print0 | sort -z | xargs -0 -r sha256sum > "$OUT_DIR/SHA256SUMS.txt"
echo 'a15_run_arc_paper=PASS'
