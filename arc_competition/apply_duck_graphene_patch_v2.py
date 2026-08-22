#!/usr/bin/env python3
from __future__ import annotations

"""Pinned-commit patch wrapper for Duck + GrapheneDMW integration.

Adds the native HypoKosh dialectic gate at Duck's real action seam while preserving
base patcher's strict pinned-source anchors and off/evidence ablations.
"""

import hashlib
import shutil
import sys
from pathlib import Path

try:
    import apply_duck_graphene_patch as base
except ModuleNotFoundError:
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    import apply_duck_graphene_patch as base

_original_replace_once = base.replace_once
_original_patch_tool_agent = base._patch_tool_agent
_original_patch_duck = base.patch_duck


def patch_tool_agent(original: str) -> str:
    text = _original_patch_tool_agent(original)
    text = _original_replace_once(
        text,
        "from inference.agent.graphene_dmw_bridge import GrapheneDMWDuckBridge\n",
        "from inference.agent.graphene_dmw_bridge import GrapheneDMWDuckBridge\n"
        "from inference.agent.graphene_dmw_native_gate import review_dialectic_action\n",
        "native-gate-import",
    )
    anchor = "            raw_payload = self._step_env_callback({\"actions\": normalized_actions})\n"
    gate = '''            if (
                self._graphene_dmw is not None
                and dmw_before_frame is not None
                and self._graphene_dmw.should_escalate_dialectic()
            ):
                dmw_native_review = review_dialectic_action(
                    state_dir=self._graphene_dmw.state_path.parent,
                    turn=int(dmw_before_frame.step),
                    action=dmw_action_label,
                    reasons=self._graphene_dmw.stagnation_reasons(),
                    scientist_note=self._graphene_dmw.state.scientist_note,
                )
                self._graphene_dmw.record_dialectic_receipt(dmw_native_review)
                if not bool(dmw_native_review.get('duck_gate_action_authorized')):
                    compact_payload = {
                        'executed': False,
                        'action_num': None,
                        'level': dmw_before_frame.level,
                        'score': None,
                        'reward': 0.0,
                        'state': 'GRAPHENE_DIALECTIC_REOPEN',
                        'valid_actions': list(valid_actions),
                        'board_changed': False,
                        'done': False,
                        'level_completed': False,
                        'game_over': False,
                        'run_complete': False,
                        'requested_count': len(normalized_actions),
                        'executed_count': 0,
                        'stopped_early': True,
                        'stop_reason': 'graphene_dialectic_reopen',
                        'stop_detail': 'Native HypoKosh selected the reopen-world-model hypothesis; re-ground and choose a discriminating alternative.',
                    }
                    self._last_action_result = dict(compact_payload)
                    return {
                        'action_result': compact_payload,
                        'state': _serialized_runtime_state(last_action_result=compact_payload),
                    }
'''
    return _original_replace_once(text, anchor, gate + anchor, "native-dialectic-action-seam")


def patch_solver(original: str) -> str:
    text = _original_replace_once(
        original,
        "    kaggle_enable_vllm: bool = field(default=True, repr=False)\n",
        "    kaggle_enable_vllm: bool = field(default=True, repr=False)\n"
        "    graphene_dmw_mode: str = field(\n"
        "        default_factory=lambda: os.environ.get('GRAPHENE_DMW_MODE', 'off').strip().lower() or 'off',\n"
        "        repr=False,\n"
        "    )\n",
        "solver-mode-field",
    )
    old_property = (
        "    @property\n"
        "    def kaggle_setup_commands(self) -> list[str]:\n"
        "        if not self.kaggle_enable_vllm:\n"
        "            return []\n"
        "        return [duck_kaggle_setup_command(self._kaggle_vllm_config())]\n"
    )
    new_property = '''    @property
    def kaggle_setup_commands(self) -> list[str]:
        mode = str(self.graphene_dmw_mode or 'off').strip().lower()
        if mode not in {'off', 'evidence', 'dialectic'}:
            raise ValueError(f'Unsupported GRAPHENE_DMW_MODE: {mode}')
        commands = []
        if self.kaggle_enable_vllm:
            commands.append(duck_kaggle_setup_command(self._kaggle_vllm_config()))
        payload = json.dumps({"GRAPHENE_DMW_MODE": mode})
        persist = (
            "\\\"$PYTHON\\\" -c 'import json,os; "
            "p=os.environ[\\\"TAAF_KAGGLE_SETUP_ENV\\\"]; "
            "d=json.load(open(p)); "
            f"d.update({payload}); "
            "json.dump(d,open(p,\\\"w\\\"),sort_keys=True)'"
        )
        commands.append(persist)
        return commands
'''
    return _original_replace_once(text, old_property, new_property, "solver-kaggle-mode-persistence")


def patch_duck(duck_root: Path, bridge_source: Path) -> dict[str, str]:
    manifest = _original_patch_duck(duck_root, bridge_source)
    gate_source = Path(__file__).resolve().with_name('graphene_dmw_native_gate.py')
    gate_target = duck_root / 'ARC3-Inference/inference/agent/graphene_dmw_native_gate.py'
    shutil.copy2(gate_source, gate_target)
    manifest['native_gate_sha256'] = hashlib.sha256(gate_source.read_bytes()).hexdigest()
    return manifest


base._patch_tool_agent = patch_tool_agent
base._patch_solver = patch_solver
base.patch_duck = patch_duck

if __name__ == "__main__":
    base.main()
