#!/usr/bin/env bash
set -euo pipefail
export PYTHONPATH="${PYTHONPATH:-}:pi45_a1"
python3 pi45_a1/a15_contract_gate.py \
  --proposal pi45_a1/a15_example_turn.json \
  --available-actions ACTION1,ACTION2 \
  --outcome pi45_a1/a15_example_outcome.json
python3 pi45_a1/a15_arc_dialectic_adapter.py \
  --mode proposal \
  --input pi45_a1/a15_example_turn.json \
  --game-id contract-smoke \
  --available-actions ACTION1,ACTION2 \
  --dry-run > /tmp/a15_proposal_request.json
python3 pi45_a1/a15_arc_dialectic_adapter.py \
  --mode outcome \
  --input pi45_a1/a15_example_outcome.json \
  --game-id contract-smoke \
  --known-hypotheses t0-h-interactable,t0-h-inert \
  --dry-run > /tmp/a15_outcome_request.json
python3 - <<'PY'
import json
p=json.load(open('/tmp/a15_proposal_request.json'))
o=json.load(open('/tmp/a15_outcome_request.json'))
assert p['dry_run'] and o['dry_run']
assert p['request']['operation']=='ingest_and_reason'
assert o['request']['operation']=='apply_outcome_and_reason'
assert p['request']['action']=='ACTION1'
assert any(n['node_type']=='Opposition' for n in p['request']['nodes'])
assert any(n['node_type']=='Experiment' for n in p['request']['nodes'])
assert o['request']['nodes'][0]['node_type']=='Outcome'
assert 'contract-smoke' not in json.dumps(p['request'])
assert 'contract-smoke' not in json.dumps(o['request'])
print('a15_adapter_smoke=PASS')
PY
