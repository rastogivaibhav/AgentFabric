#!/usr/bin/env python3
from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path
from typing import Any


def _hex(value: Any) -> str:
    return str(value).encode('utf-8').hex()


def _node(external_id: str, node_type: str, status: str, origin: str, statement: str, metadata: dict[str, Any] | None = None) -> dict[str, Any]:
    return {
        'external_id': external_id,
        'node_type': node_type,
        'status': status,
        'origin': origin,
        'statement': statement,
        'metadata': {str(k): str(v) for k, v in (metadata or {}).items()},
    }


def build_stagnation_request(*, turn: int, action: str, reasons: list[str], scientist_note: dict[str, str]) -> tuple[dict[str, Any], str, str]:
    if not reasons:
        raise ValueError('native dialectic gate requires observed stagnation evidence')
    anchor = f'duck-dmw-{turn}-comparison-anchor'
    keep = f'duck-dmw-{turn}-keep-plan'
    reopen = f'duck-dmw-{turn}-reopen-model'
    observation = f'duck-dmw-{turn}-stagnation'
    reason_text = ', '.join(sorted(set(reasons)))
    plan = ' '.join((scientist_note.get('current_plan') or scientist_note.get('world_model') or 'current working plan').split())
    question = ' '.join((scientist_note.get('open_questions') or 'Which alternative interaction best discriminates the current world-model hypotheses?').split())
    n = len(set(reasons))
    # Evidence-derived, domain-agnostic confidence: more independent stagnation signals
    # reduce support for perseverating and increase support for reopening the model.
    keep_support = max(0.15, 0.65 - 0.20 * n)
    reopen_support = min(0.95, 0.55 + 0.15 * n)
    nodes = [
        _node(anchor, 'Outcome', 'Proposed', 'Hypothetical', 'Choose whether the currently proposed action remains epistemically justified or the world model should be reopened.', {'kind':'duck_dialectic_comparison'}),
        _node(observation, 'Fact', 'Active', 'Observed', f'Observed stagnation evidence: {reason_text}.', {'kind':'stagnation_evidence','reason_count':n}),
        _node(keep, 'Hypothesis', 'Proposed', 'Hypothetical', f'The current plan remains valid and proposed action {action} is still the best discriminating test. Plan: {plan}', {'kind':'continue_current_plan'}),
        _node(reopen, 'Hypothesis', 'Proposed', 'Hypothetical', f'The current interpretation is in false convergence; withhold repeated commitment and reopen alternatives before action {action}. Open question: {question}', {'kind':'reopen_world_model'}),
    ]
    relations = [
        {'from': keep, 'to': anchor, 'role': 'Supports', 'origin': 'Hypothetical', 'confidence': keep_support},
        {'from': reopen, 'to': anchor, 'role': 'Supports', 'origin': 'Hypothetical', 'confidence': reopen_support},
        {'from': observation, 'to': keep, 'role': 'Contradicts', 'origin': 'Observed', 'confidence': min(0.95, 0.55 + 0.10*n)},
        {'from': observation, 'to': reopen, 'role': 'Supports', 'origin': 'Observed', 'confidence': min(0.95, 0.65 + 0.10*n)},
    ]
    request = {
        'protocol': 'agentfabric-duck-dmw-native-v1',
        'operation': 'ingest_and_reason',
        'world_scope': os.environ.get('GRAPHENE_DMW_WORLD_SCOPE', 'duck-arc3-world'),
        'turn': int(turn),
        'nodes': nodes,
        'relations': relations,
        'action': action,
    }
    return request, keep, reopen


def native_line_protocol(request: dict[str, Any]) -> str:
    lines = ['A15V1', f"OP {_hex(request['operation'])}", f"GAME {_hex(request['world_scope'])}", f"TURN {int(request['turn'])}"]
    for n in request.get('nodes') or []:
        lines.append(' '.join(['NODE', _hex(n['external_id']), n['node_type'], n['status'], n['origin'], _hex(n['statement']), _hex(json.dumps(n.get('metadata') or {}, sort_keys=True, separators=(',',':')))]))
    for r in request.get('relations') or []:
        lines.append(' '.join(['REL', _hex(r['from']), _hex(r['to']), r['role'], r['origin'], format(float(r['confidence']), '.17g')]))
    lines.append(f"ACTION {_hex(request['action'])}")
    lines.append('END')
    return '\n'.join(lines) + '\n'


def _ensure_db(db_path: Path, bootstrap: str) -> None:
    if db_path.exists():
        return
    db_path.parent.mkdir(parents=True, exist_ok=True)
    proc = subprocess.run([bootstrap, str(db_path)], text=True, capture_output=True, timeout=30)
    if proc.returncode != 0:
        raise RuntimeError(f'Graphene bootstrap failed rc={proc.returncode}: {proc.stderr.strip()}')


def review_dialectic_action(*, state_dir: str | Path, turn: int, action: str, reasons: list[str], scientist_note: dict[str, str]) -> dict[str, Any]:
    helper = os.environ.get('GRAPHENE_DMW_NATIVE_HELPER', '').strip()
    bootstrap = os.environ.get('GRAPHENE_DMW_BOOTSTRAP', '').strip()
    if not helper or not bootstrap:
        raise RuntimeError('dialectic mode requires GRAPHENE_DMW_NATIVE_HELPER and GRAPHENE_DMW_BOOTSTRAP')
    request, keep, reopen = build_stagnation_request(turn=turn, action=action, reasons=reasons, scientist_note=scientist_note)
    db_path = Path(state_dir) / 'graphene_dmw_native.graphenedb'
    _ensure_db(db_path, bootstrap)
    proc = subprocess.run([helper, '--db', str(db_path)], input=native_line_protocol(request), text=True, capture_output=True, timeout=60)
    if proc.returncode != 0:
        raise RuntimeError(f'native HypoKosh gate failed rc={proc.returncode}: {proc.stderr.strip()}')
    response = json.loads(proc.stdout)
    receipt = response.get('reasoning_receipt') or {}
    required = ['graphene_executed','fiber_bundle_built','stability_critic_executed','epistemic_admissibility_executed','lyapunov_trajectory_executed','convergence_executed','opposition_executed','no_silent_promotion']
    if not all(bool(receipt.get(k)) for k in required):
        raise RuntimeError(f'native HypoKosh reasoning incomplete: {receipt}')
    primary = str(response.get('primary_hypothesis_id') or '')
    if primary not in {keep, reopen}:
        raise RuntimeError(f'native HypoKosh selected no current Duck gate hypothesis: {primary!r}')
    return {
        **response,
        'duck_gate_decision': 'continue-current-plan' if primary == keep else 'reopen-world-model',
        'duck_gate_action_authorized': primary == keep,
        'duck_gate_request': request,
    }
