#!/usr/bin/env bash
set -euo pipefail

GAME="${1:?game required}"
MAX_ACTIONS="${2:?max actions required}"
OUT_DIR="${3:?out dir required}"
MEMORY_WINDOW=6
CONTEXT_SIZE=8192
PROMPT_BUDGET_CHARS=8000
UPSTREAM_TIMEOUT_S=360
CLIENT_TIMEOUT_FLOOR_S=420
PRIOR_COUNT=8
GRAPHENE_HELPER=/tmp/pi45a1/build/pi45_graphene_memory
MODEL=/tmp/qwen2.5-1.5b-instruct-q4_k_m.gguf
PRIOR_DB="$OUT_DIR/a14_universal_prior.graphenedb"
PRIOR_MANIFEST="$OUT_DIR/a14_prior_seed_manifest.json"
PRIOR_RETRIEVAL_LOG="$OUT_DIR/a14_prior_retrieval.jsonl"
TREATMENT_SCRIPT=/tmp/pi45a14/arc_a14_graphene_prior.py

mkdir -p "$OUT_DIR" /tmp/pi45a14
cp /tmp/model_SHA256SUMS.txt "$OUT_DIR/model_SHA256SUMS.txt"
cp /tmp/graphenedb_commit.txt "$OUT_DIR/graphenedb_commit.txt"
cp /tmp/graphenedb_source_mode.txt "$OUT_DIR/graphenedb_source_mode.txt"
cp /tmp/graphenedb_source_manifest_SHA256SUMS.txt "$OUT_DIR/graphenedb_source_manifest_SHA256SUMS.txt"
cp "$GITHUB_WORKSPACE/pi45_a1/graphenedb_snapshot/SOURCE_MANIFEST.txt" "$OUT_DIR/graphenedb_SOURCE_MANIFEST.txt"
sha256sum "$GITHUB_WORKSPACE/pi45_a1/transport_safe_chat_proxy.py" > "$OUT_DIR/transport_safe_chat_proxy_SHA256SUMS.txt"
sha256sum "$GITHUB_WORKSPACE/pi45_a1/transport_safe_runner.py" > "$OUT_DIR/transport_safe_runner_SHA256SUMS.txt"
sha256sum "$GITHUB_WORKSPACE/pi45_a1/a14_prior.json" > "$OUT_DIR/a14_prior_SHA256SUMS.txt"
sha256sum "$GITHUB_WORKSPACE/pi45_a1/a14_seed_graphene.py" > "$OUT_DIR/a14_seed_graphene_SHA256SUMS.txt"
sha256sum "$GITHUB_WORKSPACE/pi45_a1/a14_build_treatment.py" > "$OUT_DIR/a14_build_treatment_SHA256SUMS.txt"
sha256sum /tmp/pi45a1/arc_a1_graphene_episodic.py > "$OUT_DIR/frozen_a1_script_SHA256SUMS.txt"
git -C /tmp/llama.cpp rev-parse HEAD > "$OUT_DIR/llama_cpp_commit.txt"

printf '%s\n' \
  "game=${GAME}" \
  "max_actions=${MAX_ACTIONS}" \
  "memory_window=${MEMORY_WINDOW}" \
  "context_size=${CONTEXT_SIZE}" \
  "prompt_budget_chars=${PROMPT_BUDGET_CHARS}" \
  "upstream_timeout_s=${UPSTREAM_TIMEOUT_S}" \
  "client_timeout_floor_s=${CLIENT_TIMEOUT_FLOOR_S}" \
  "llama_parallel=1" \
  "graphenedb_mode=episodic_nodes_vector_search_only_plus_separate_universal_prior_store" \
  "a14_prior_name=minimal-interactive-world-prior-v1" \
  "a14_prior_memory_count=${PRIOR_COUNT}" \
  "a14_prior_categories=controller_goal_interaction-design_game-design" \
  "a14_target_game_specific_prior=false" \
  "a14_context_policy=head_tail_deterministic_bound" \
  "a14_transport_policy=single_inflight_extended_local_timeout" \
  "a14_validity_gate=llm_errors_zero_transport_clean_prior_retrieved_every_turn" > "$OUT_DIR/treatment_contract.txt"

echo '63f205b3dbbad0d580dc8a262edbf8dd432c3ce6df173e0f888ac2188eeb78d1  /tmp/pi45a1/arc_a1_graphene_episodic.py' | sha256sum -c -

python3 "$GITHUB_WORKSPACE/pi45_a1/a14_seed_graphene.py" \
  --prior "$GITHUB_WORKSPACE/pi45_a1/a14_prior.json" \
  --helper "$GRAPHENE_HELPER" \
  --db "$PRIOR_DB" \
  --manifest "$PRIOR_MANIFEST"

python3 "$GITHUB_WORKSPACE/pi45_a1/a14_build_treatment.py" \
  --frozen /tmp/pi45a1/arc_a1_graphene_episodic.py \
  --output "$TREATMENT_SCRIPT"
python3 -m py_compile "$TREATMENT_SCRIPT"
sha256sum "$TREATMENT_SCRIPT" > "$OUT_DIR/a14_treatment_script_SHA256SUMS.txt"

/tmp/llama.cpp/build/bin/llama-server \
  -m "$MODEL" -c "$CONTEXT_SIZE" --parallel 1 \
  --host 127.0.0.1 --port 9091 --alias pi45a1-qwen25-15b \
  > "$OUT_DIR/llama_server.log" 2>&1 &
SERVER_PID=$!

python3 "$GITHUB_WORKSPACE/pi45_a1/transport_safe_chat_proxy.py" \
  --listen 9090 --target http://127.0.0.1:9091 \
  --budget-chars "$PROMPT_BUDGET_CHARS" \
  --upstream-timeout "$UPSTREAM_TIMEOUT_S" \
  --log "$OUT_DIR/transport_proxy.jsonl" \
  > "$OUT_DIR/transport_proxy.log" 2>&1 &
PROXY_PID=$!
trap 'kill "$PROXY_PID" "$SERVER_PID" 2>/dev/null || true' EXIT

ready=0
for i in $(seq 1 120); do
  if curl -fsS http://127.0.0.1:9090/health >/dev/null 2>&1; then ready=1; break; fi
  sleep 1
done
test "$ready" = 1

set +e
A14_PRIOR_DB="$PRIOR_DB" \
A14_PRIOR_RETRIEVAL_LOG="$PRIOR_RETRIEVAL_LOG" \
A14_PRIOR_COUNT="$PRIOR_COUNT" \
/tmp/pi45venv/bin/python "$GITHUB_WORKSPACE/pi45_a1/transport_safe_runner.py" \
  --client-timeout-floor "$CLIENT_TIMEOUT_FLOOR_S" \
  --script "$TREATMENT_SCRIPT" \
  --games "$GAME" --max-actions "$MAX_ACTIONS" --memory-window "$MEMORY_WINDOW" \
  --graphene-helper "$GRAPHENE_HELPER" --out "$OUT_DIR" \
  2>&1 | tee "$OUT_DIR/agent_stdout.log"
AGENT_RC=${PIPESTATUS[0]}
set -e
printf '%s\n' "$AGENT_RC" > "$OUT_DIR/agent_exit_code.txt"
test "$AGENT_RC" -eq 0
test -f "$OUT_DIR/summary.json"

OUT_DIR="$OUT_DIR" PROMPT_BUDGET_CHARS="$PROMPT_BUDGET_CHARS" CLIENT_TIMEOUT_FLOOR_S="$CLIENT_TIMEOUT_FLOOR_S" PRIOR_COUNT="$PRIOR_COUNT" python3 - <<'PY'
import json, math, os, re
from pathlib import Path

out = Path(os.environ['OUT_DIR'])
x = json.load(open(out / 'summary.json'))
assert x['graphenedb_enabled'] is True
assert x['graphenedb_mode'] == 'episodic_nodes_vector_search_only'
assert x['persistent_world_model'] is False
assert x['causal_edges_enabled'] is False
assert x['hex_fiber_reasoning_enabled'] is False
assert x['epistemic_gate_enabled'] is False
assert x['model_sha256'] == '6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e'
for g in x['games']:
    st = g.get('graphenedb_stats') or {}
    assert st.get('valid') is True, (g.get('game_id'), st)
    assert st.get('edges') == 0, (g.get('game_id'), st)
    inst = g.get('instrumentation') or {}
    assert inst.get('llm_errors', 0) == 0, (g.get('game_id'), inst)

prior_manifest = json.load(open(out / 'a14_prior_seed_manifest.json'))
assert prior_manifest['prior_memory_count'] == int(os.environ['PRIOR_COUNT'])
assert prior_manifest['target_game_specific'] is False
assert prior_manifest['stats']['valid'] is True
assert prior_manifest['stats']['edges'] == 0
assert prior_manifest['stats']['nodes'] == int(os.environ['PRIOR_COUNT'])

prior_records = [json.loads(line) for line in open(out / 'a14_prior_retrieval.jsonl') if line.strip()]
actions = int(x['aggregate']['actions'])
assert len(prior_records) == actions, (len(prior_records), actions)
assert all(r.get('source') == 'graphenedb_vector_search' for r in prior_records)
assert all(int(r.get('prior_hits', 0)) == int(os.environ['PRIOR_COUNT']) for r in prior_records)
expected_ids = {
    'controller-gameboy', 'controller-nes', 'controller-snes', 'controller-xbox',
    'controller-pc', 'goal-setting', 'interaction-design', 'game-design'
}
assert all(set(r.get('prior_ids') or []) == expected_ids for r in prior_records)

records = [json.loads(line) for line in open(out / 'transport_proxy.jsonl') if line.strip()]
posts = [r for r in records if r.get('method') == 'POST']
assert posts, 'no proxied inference requests recorded'
budget = int(os.environ['PROMPT_BUDGET_CHARS'])
assert all(r.get('bounded_chars', 0) <= budget for r in posts)
assert max(r.get('max_active_observed', 0) for r in posts) <= 1, posts
assert not any(r.get('client_disconnected') for r in posts), posts
assert not any((r.get('status') or 0) >= 500 for r in posts), posts
assert not any(r.get('error') for r in posts), posts

lat = sorted(float(r.get('upstream_elapsed_s', 0)) for r in posts)
def percentile(values, p):
    if not values:
        return 0.0
    idx = max(0, min(len(values)-1, math.ceil(p * len(values)) - 1))
    return values[idx]

stdout = (out / 'agent_stdout.log').read_text(encoding='utf-8', errors='replace')
matches = re.findall(r'a13_transport_runner_stats=eligible_calls=(\d+) timeouts_raised=(\d+) max_original_timeout_s=([0-9.]+)', stdout)
assert matches, 'transport runner stats missing'
eligible, raised, original = matches[-1]
eligible, raised, original = int(eligible), int(raised), float(original)
assert eligible > 0

transport = {
    'requests': len(posts),
    'truncated_requests': sum(bool(r.get('truncated')) for r in posts),
    'max_original_chars': max(r.get('original_chars', 0) for r in posts),
    'max_bounded_chars': max(r.get('bounded_chars', 0) for r in posts),
    'budget_chars': budget,
    'max_concurrent_inference_requests': max(r.get('max_active_observed', 0) for r in posts),
    'client_disconnects': sum(bool(r.get('client_disconnected')) for r in posts),
    'transport_errors': sum(bool(r.get('error')) or (r.get('status') or 0) >= 500 for r in posts),
    'max_queue_s': max(float(r.get('queue_s', 0)) for r in posts),
    'p50_upstream_s': percentile(lat, 0.50),
    'p95_upstream_s': percentile(lat, 0.95),
    'max_upstream_s': max(lat),
    'client_timeout_floor_s': float(os.environ['CLIENT_TIMEOUT_FLOOR_S']),
    'client_timeout_eligible_calls': eligible,
    'client_timeouts_raised': raised,
    'max_original_client_timeout_s': original,
}
json.dump(transport, open(out / 'transport_metrics.json', 'w'), indent=2, sort_keys=True)

turn_files = list(out.glob('*.turns.jsonl'))
turns = []
if turn_files:
    turns = [json.loads(line) for line in open(turn_files[0]) if line.strip()]
first_change_turn = next((int(t['turn']) for t in turns if (not bool(t.get('no_op', False))) or int(t.get('levels_after', 0)) > int(t.get('levels_before', 0))), None)
game = x['games'][0]
behavior = {
    'game_id': game.get('game_id'),
    'score': (x.get('scorecard') or {}).get('score'),
    'levels_gained': game.get('levels_gained'),
    'actions': game.get('actions'),
    'action_counts': game.get('action_counts') or {},
    'unique_actions': len(game.get('action_counts') or {}),
    'no_ops': x['aggregate'].get('no_ops', 0),
    'repeated_state_action': x['aggregate'].get('repeated_state_action', 0),
    'first_meaningful_change_turn': first_change_turn,
    'prior_queries': len(prior_records),
    'prior_hits_per_query': int(os.environ['PRIOR_COUNT']),
}
json.dump(behavior, open(out / 'a14_behavior_metrics.json', 'w'), indent=2, sort_keys=True)
print('pi45a14_prior_contract=PASS')
print(json.dumps(behavior, indent=2, sort_keys=True))
print(json.dumps(transport, indent=2, sort_keys=True))
PY
