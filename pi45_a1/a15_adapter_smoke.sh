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
req=p['request']
assert req['operation']=='ingest_and_reason'
assert o['request']['operation']=='apply_outcome_and_reason'
assert req['action']=='ACTION1'
assert any(n['node_type']=='Opposition' for n in req['nodes'])
assert any(n['node_type']=='Experiment' for n in req['nodes'])
assert o['request']['nodes'][0]['node_type']=='Outcome'
assert req['candidate_hypothesis_ids']==['t0-h-interactable','t0-h-inert']
for forbidden in ['provisional_hypothesis_id','provisional_goal_id','reopen_hypothesis_ids']:
    assert forbidden not in req, forbidden
assert not any(r['from']=='turn-0-opposition' for r in req['relations'])
assert 'contract-smoke' not in json.dumps(req)
assert 'contract-smoke' not in json.dumps(o['request'])
print('a15_adapter_smoke=PASS')
PY
