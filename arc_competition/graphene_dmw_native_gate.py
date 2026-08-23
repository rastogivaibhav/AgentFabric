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


def build_goal_discovery_request(*, turn: int, action: str, evidence: dict[str, Any]) -> tuple[dict[str, Any], dict[str, str]]:
    """Build a DMW goal-discovery competition from observed interaction evidence.

    The hypotheses are deliberately generic and provisional. The native runtime
    receives observed support/contradiction edges; it is not told a winning goal.
    """
    observed = max(0, int(evidence.get('observations') or 0))
    changed = max(0, int(evidence.get('changed') or 0))
    level = max(0, int(evidence.get('level_transitions') or 0))
    silent = max(0, int(evidence.get('silent') or 0))
    delayed = max(0, int(evidence.get('delayed_effects') or 0))
    anchor = f'duck-goal-{turn}-anchor'
    obs = f'duck-goal-{turn}-evidence'
    ids = {
        'progression': f'duck-goal-{turn}-progression',
        'causal_novelty': f'duck-goal-{turn}-causal-novelty',
        'latent_sequence': f'duck-goal-{turn}-latent-sequence',
    }
    denom = max(1, observed)
    progression_support = min(0.98, 0.20 + 0.75 * (level / denom))
    novelty_support = min(0.90, 0.25 + 0.60 * (changed / denom))
    latent_support = min(0.92, 0.20 + 0.35 * (silent / denom) + 0.35 * (delayed / denom))
    nodes = [
        _node(anchor, 'Outcome', 'Proposed', 'Hypothetical', 'Infer which provisional goal model currently best explains observed ARC interaction evidence and should guide the next discriminating experiment.', {'kind':'dialectical_goal_discovery'}),
        _node(obs, 'Fact', 'Active', 'Observed', f'Crawler evidence: observations={observed}, changed={changed}, level_transitions={level}, silent={silent}, delayed_effects={delayed}.', {'kind':'goal_evidence','observations':observed,'changed':changed,'level_transitions':level,'silent':silent,'delayed_effects':delayed}),
        _node(ids['progression'], 'Hypothesis', 'Proposed', 'Hypothetical', 'G1 progression: interactions that cause durable level/state progression are closest to the latent objective.', {'kind':'goal_hypothesis','goal_kind':'progression'}),
        _node(ids['causal_novelty'], 'Hypothesis', 'Proposed', 'Hypothetical', 'G2 causal novelty: novel persistent state changes identify mechanisms that may define or unlock the latent objective.', {'kind':'goal_hypothesis','goal_kind':'causal_novelty'}),
        _node(ids['latent_sequence'], 'Hypothesis', 'Proposed', 'Hypothetical', 'G3 latent sequence: apparently silent interactions can be necessary preconditions in delayed multi-step objective-reaching sequences.', {'kind':'goal_hypothesis','goal_kind':'latent_sequence'}),
    ]
    relations = [
        {'from': ids['progression'], 'to': anchor, 'role': 'Supports', 'origin': 'Hypothetical', 'confidence': progression_support},
        {'from': ids['causal_novelty'], 'to': anchor, 'role': 'Supports', 'origin': 'Hypothetical', 'confidence': novelty_support},
        {'from': ids['latent_sequence'], 'to': anchor, 'role': 'Supports', 'origin': 'Hypothetical', 'confidence': latent_support},
        {'from': obs, 'to': ids['progression'], 'role': 'Supports' if level else 'Contradicts', 'origin': 'Observed', 'confidence': 0.95 if level else min(0.65, 0.30 + 0.05 * observed)},
        {'from': obs, 'to': ids['causal_novelty'], 'role': 'Supports' if changed else 'Contradicts', 'origin': 'Observed', 'confidence': min(0.90, 0.45 + 0.45 * (changed / denom))},
        {'from': obs, 'to': ids['latent_sequence'], 'role': 'Supports' if (silent or delayed) else 'Contradicts', 'origin': 'Observed', 'confidence': min(0.90, 0.40 + 0.25 * (silent / denom) + 0.25 * (delayed / denom))},
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
    return request, ids


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


def _run_native(*, state_dir: str | Path, request: dict[str, Any]) -> dict[str, Any]:
    helper = os.environ.get('GRAPHENE_DMW_NATIVE_HELPER', '').strip()
    bootstrap = os.environ.get('GRAPHENE_DMW_BOOTSTRAP', '').strip()
    if not helper or not bootstrap:
        raise RuntimeError('dialectic mode requires GRAPHENE_DMW_NATIVE_HELPER and GRAPHENE_DMW_BOOTSTRAP')
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
    return response


def review_goal_discovery(*, state_dir: str | Path, turn: int, action: str, evidence: dict[str, Any]) -> dict[str, Any]:
    request, ids = build_goal_discovery_request(turn=turn, action=action, evidence=evidence)
    response = _run_native(state_dir=state_dir, request=request)
    primary = str(response.get('primary_hypothesis_id') or '')
    reverse = {v: k for k, v in ids.items()}
    if primary not in reverse:
        raise RuntimeError(f'native HypoKosh selected no current goal hypothesis: {primary!r}')
    return {
        **response,
        'goal_discovery_selected': reverse[primary],
        'goal_discovery_hypothesis_id': primary,
        'goal_discovery_request': request,
    }


def review_dialectic_action(*, state_dir: str | Path, turn: int, action: str, reasons: list[str], scientist_note: dict[str, str]) -> dict[str, Any]:
    request, keep, reopen = build_stagnation_request(turn=turn, action=action, reasons=reasons, scientist_note=scientist_note)
    response = _run_native(state_dir=state_dir, request=request)
    primary = str(response.get('primary_hypothesis_id') or '')
    if primary not in {keep, reopen}:
        raise RuntimeError(f'native HypoKosh selected no current Duck gate hypothesis: {primary!r}')
    return {
        **response,
        'duck_gate_decision': 'continue-current-plan' if primary == keep else 'reopen-world-model',
        'duck_gate_action_authorized': primary == keep,
        'duck_gate_request': request,
    }
