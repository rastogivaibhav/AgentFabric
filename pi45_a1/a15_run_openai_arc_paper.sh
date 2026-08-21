#!/usr/bin/env bash
set -euo pipefail

GAME="${1:-ft09}"
MAX_ACTIONS="${2:-10}"
OUT_DIR="${3:?out dir required}"
ROOT="${GITHUB_WORKSPACE:-$(pwd)}"
RUNTIME="$ROOT/pi45_a1/a15_native_runtime"
MODEL_WORLD="$ROOT/pi45_a1/a15_native_model_world"
CORE="$ROOT/pi45_a1/graphenedb_snapshot"
HELPER=/tmp/a15_native_runtime_helper
BOOTSTRAP=/tmp/a15_graphene_bootstrap
DUMPER=/tmp/a15_model_world_dump
OPENAI_MODEL="${OPENAI_MODEL:-gpt-5.6-sol}"

# Never echo, print, hash, persist, or pass the secret as a CLI argument.
test -n "${OPENAI_API_KEY:-}" || {
  echo 'OPENAI_API_KEY secret is not configured' >&2
  exit 78
}

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

python3 -m py_compile \
  "$ROOT/pi45_a1/a15_arc_diagnostic_runner.py" \
  "$ROOT/pi45_a1/a15_arc_paper_runner.py" \
  "$ROOT/pi45_a1/a15_openai_arc_paper_runner.py" \
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

# Same native GrapheneDB / HypoKosh binaries as frozen A1.5.
g++ -std=c++20 -O2 -UNDEBUG -I"$CORE/include" \
  "$ROOT/pi45_a1/a15_graphene_bootstrap.cpp" \
  "$CORE/src/db.cpp" "$CORE/src/platform_posix.cpp" \
  -pthread -o "$BOOTSTRAP"

g++ -std=c++20 -O2 -UNDEBUG \
  -I"$RUNTIME/include" -I"$MODEL_WORLD/include" -I"$CORE/include" \
  "$ROOT/pi45_a1/a15_native_runtime_helper.cpp" \
  "$CORE/src/db.cpp" "$CORE/src/platform_posix.cpp" \
  "$MODEL_WORLD/src/model_world.cpp" \
  "$RUNTIME/src/epistemic.cpp" "$RUNTIME/src/dialectic.cpp" \
  "$RUNTIME/src/fiber_bundle.cpp" "$RUNTIME/src/stability_critic.cpp" \
  "$RUNTIME/src/epistemic_control.cpp" "$RUNTIME/src/escape.cpp" \
  "$RUNTIME/src/self_healing.cpp" "$RUNTIME/src/path_verifier.cpp" \
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
  "$ROOT/pi45_a1/a15_openai_arc_paper_runner.py" \
  "$ROOT/pi45_a1/a15_arc_dialectic_adapter.py" \
  "$ROOT/pi45_a1/a15_proposal_prompt.py" \
  "$ROOT/pi45_a1/a15_outcome_prompt.py" \
  "$ROOT/pi45_a1/a15_contract_gate.py" \
  "$ROOT/pi45_a1/a15_state_contract.json" \
  "$ROOT/pi45_a1/a15_reasoning_constitution.json" \
  > "$OUT_DIR/treatment_sources_SHA256SUMS.txt"

printf '%s\n' \
  'experiment=PI4.5A1.5-openai-capacity-control' \
  "game=$GAME" \
  "max_actions=$MAX_ACTIONS" \
  'provider=openai' \
  "model=$OPENAI_MODEL" \
  'api=responses' \
  'store=false' \
  'key_exposed_to_artifacts=false' \
  'perception=structured-grid-plus-opaque-actions' \
  'observation_authority=deterministic-runtime' \
  'proposal_contract=frozen-a15-v5' \
  'dialectic_control_owner=CompleteHypoKoshRuntime' \
  'persistent_model_world=true' \
  'architecture_change=none' \
  'independent_variable=proposal-model-capacity' \
  > "$OUT_DIR/paper_treatment_contract.txt"

export OPENAI_MODEL
export OPENAI_CALL_LOG="$OUT_DIR/openai_calls.jsonl"
set +e
PYTHONPATH="$ROOT/pi45_a1" /tmp/pi45venv/bin/python "$ROOT/pi45_a1/a15_openai_arc_paper_runner.py" \
  --games "$GAME" --max-actions "$MAX_ACTIONS" \
  --endpoint openai-capacity-control://responses \
  --out "$OUT_DIR" \
  --native-helper "$HELPER" --bootstrap "$BOOTSTRAP" --modelworld-dump "$DUMPER" \
  --contract-path "$ROOT/pi45_a1/a15_state_contract.json" \
  2>&1 | tee "$OUT_DIR/runner_stdout.log"
RUNNER_RC=${PIPESTATUS[0]}
set -e
printf '%s\n' "$RUNNER_RC" > "$OUT_DIR/runner_exit_code.txt"
test "$RUNNER_RC" -eq 0
test -f "$OUT_DIR/summary.json"

OUT_DIR="$OUT_DIR" GAME="$GAME" MAX_ACTIONS="$MAX_ACTIONS" OPENAI_MODEL="$OPENAI_MODEL" python3 - <<'PY'
import json, os
from pathlib import Path
out=Path(os.environ['OUT_DIR'])
s=json.loads((out/'summary.json').read_text())
g=(s.get('games') or [{}])[0]
inst=g.get('instrumentation') or {}
invalid=[]
if s.get('model_id') != os.environ['OPENAI_MODEL']:
    invalid.append('model_id_mismatch')
if s.get('evaluator_metadata_in_reasoning') is not False:
    invalid.append('evaluator_metadata_flag')
if s.get('native_dialectical_runtime') is not True:
    invalid.append('native_runtime_flag')
if s.get('persistent_model_world') is not True:
    invalid.append('persistent_model_world_flag')
if g.get('error'):
    invalid.append('environment:'+str(g.get('error')))
for key in ['native_errors','step_errors','null_steps','empty_action_space']:
    if int(inst.get(key,0)):
        invalid.append(f'{key}={inst.get(key)}')

# Verify evaluator-only metadata does not enter epistemic records.
for path in out.glob('*.a15.turns.jsonl'):
    for line in path.read_text().splitlines():
        if not line.strip(): continue
        row=json.loads(line)
        epistemic={k:v for k,v in row.items() if k!='evaluator'}
        encoded=json.dumps(epistemic,sort_keys=True).lower()
        for forbidden in ['game_id','win_levels','levels_completed','score_delta','level_delta','scorecard','official_success','level_scores']:
            if forbidden in encoded:
                invalid.append(f'evaluator_leak:{forbidden}:turn={row.get("turn")}')

calls=[]
call_path=out/'openai_calls.jsonl'
if call_path.exists():
    calls=[json.loads(x) for x in call_path.read_text().splitlines() if x.strip()]
if not calls:
    # Zero calls is allowed only if the environment fails before proposal; that
    # case is already invalid. A valid treatment must actually exercise OpenAI.
    invalid.append('no_openai_calls')
api_errors=sum(1 for x in calls if x.get('error') or int(x.get('status') or 0) >= 500)
if api_errors:
    invalid.append(f'openai_api_errors={api_errors}')
metrics={
  'provider':'openai',
  'model':os.environ['OPENAI_MODEL'],
  'valid_measurement':not invalid,
  'invalid_reasons':sorted(set(invalid)),
  'game_id':g.get('game_id') or os.environ['GAME'],
  'actions':int(g.get('actions',0)),
  'max_actions':int(os.environ['MAX_ACTIONS']),
  'levels_gained':int(g.get('levels_gained',0)),
  'levels_completed':int(g.get('levels_completed',0)),
  'terminal_state':g.get('terminal_state'),
  'action_counts':g.get('action_counts') or {},
  'unique_actions':int(g.get('unique_actions',0)),
  'no_ops':int(inst.get('no_ops',0)),
  'repeated_state_action':int(inst.get('repeated_state_action',0)),
  'proposal_errors':int(inst.get('proposal_errors',0)),
  'outcome_errors':int(inst.get('outcome_errors',0)),
  'hypotheses_generated':int(inst.get('hypotheses_generated',0)),
  'candidate_goals_generated':int(inst.get('candidate_goals_generated',0)),
  'hypotheses_supported':int(inst.get('hypotheses_supported',0)),
  'hypotheses_contradicted':int(inst.get('hypotheses_contradicted',0)),
  'lyapunov_checks':int(inst.get('lyapunov_checks',0)),
  'escape_considered':int(inst.get('escape_considered',0)),
  'model_world_nodes':int(g.get('model_world_nodes',0)),
  'model_world_event_hash':int(g.get('model_world_event_hash',0)),
  'paper_wrapper':s.get('paper_wrapper') or {},
  'openai_requests':len(calls),
  'openai_api_errors':api_errors,
  'openai_max_latency_s':max((float(x.get('elapsed_s') or 0) for x in calls),default=0),
  'openai_usage': [x.get('usage') or {} for x in calls],
}
json.dump({'valid_measurement':not invalid,'invalid_reasons':sorted(set(invalid))},open(out/'measurement_validity.json','w'),indent=2,sort_keys=True)
json.dump(metrics,open(out/'openai_paper_result.json','w'),indent=2,sort_keys=True)
print(json.dumps(metrics,indent=2,sort_keys=True))
if invalid:
    raise SystemExit(3)
PY

find "$OUT_DIR" -type f ! -name SHA256SUMS.txt -print0 | sort -z | xargs -0 -r sha256sum > "$OUT_DIR/SHA256SUMS.txt"
echo 'a15_run_openai_arc_paper=PASS'
