#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import shutil
from pathlib import Path

UPSTREAM_COMMIT = "7652836056c59e044f093e3c13ed7438c814169e"


def replace_once(text: str, old: str, new: str, label: str) -> str:
    count = text.count(old)
    if count != 1:
        raise RuntimeError(f"Duck patch anchor {label!r} expected once, found {count}")
    return text.replace(old, new, 1)


def _patch_tool_agent(original: str) -> str:
    text = replace_once(
        original,
        "from inference.agent.runtime_state import Frame, HistoryEntry, RUNTIME_STATE_FILENAME, load_runtime_state\n",
        "from inference.agent.runtime_state import Frame, HistoryEntry, RUNTIME_STATE_FILENAME, load_runtime_state\n"
        "from inference.agent.graphene_dmw_bridge import GrapheneDMWDuckBridge\n",
        "import",
    )
    text = replace_once(
        text,
        "        self._summarized_knowledge = _empty_world_model()\n",
        "        self._summarized_knowledge = _empty_world_model()\n"
        "        self._graphene_dmw: GrapheneDMWDuckBridge | None = None\n",
        "constructor-state",
    )
    text = replace_once(
        text,
        "            self._summarized_knowledge = _empty_world_model()\n",
        "            self._summarized_knowledge = _empty_world_model()\n"
        "            dmw_mode = os.environ.get('GRAPHENE_DMW_MODE', 'off').strip().lower() or 'off'\n"
        "            self._graphene_dmw = GrapheneDMWDuckBridge(runtime_dir / 'graphene_dmw_state.json', mode=dmw_mode)\n",
        "session-init",
    )
    text = replace_once(
        text,
        "        for key, value in note.items():\n            if value:\n                self._summarized_knowledge[key] = value\n",
        "        for key, value in note.items():\n            if value:\n                self._summarized_knowledge[key] = value\n"
        "        if self._graphene_dmw is not None:\n"
        "            self._graphene_dmw.ingest_scientist_note(note)\n",
        "scientist-note",
    )
    text = replace_once(
        text,
        "        lines.extend(self._summarized_knowledge_lines())\n        lines.append(\"end of world model. \")\n",
        "        lines.extend(self._summarized_knowledge_lines())\n"
        "        if self._graphene_dmw is not None:\n"
        "            dmw_context = self._graphene_dmw.prompt_context()\n"
        "            if dmw_context:\n"
        "                lines.extend(['', dmw_context])\n"
        "        lines.append(\"end of world model. \")\n",
        "prompt-context",
    )
    old_step_block = (
        "            raw_payload = self._step_env_callback({\"actions\": normalized_actions})\n"
        "            if not isinstance(raw_payload, dict):\n"
    )
    new_step_block = (
        "            dmw_before_frame, _ = load_runtime_state(state_path)\n"
        "            dmw_action_label = (\n"
        "                self._graphene_dmw.canonical_action(normalized_actions)\n"
        "                if self._graphene_dmw is not None else ''\n"
        "            )\n"
        "            dmw_block_reason = (\n"
        "                self._graphene_dmw.dead_action_reason(dmw_before_frame.grid, dmw_action_label)\n"
        "                if self._graphene_dmw is not None and dmw_before_frame is not None else None\n"
        "            )\n"
        "            if dmw_block_reason:\n"
        "                compact_payload = {\n"
        "                    'executed': False,\n"
        "                    'action_num': None,\n"
        "                    'level': dmw_before_frame.level if dmw_before_frame is not None else None,\n"
        "                    'score': None,\n"
        "                    'reward': 0.0,\n"
        "                    'state': 'GRAPHENE_EVIDENCE_BLOCK',\n"
        "                    'valid_actions': list(valid_actions),\n"
        "                    'board_changed': False,\n"
        "                    'done': False,\n"
        "                    'level_completed': False,\n"
        "                    'game_over': False,\n"
        "                    'run_complete': False,\n"
        "                    'requested_count': len(normalized_actions),\n"
        "                    'executed_count': 0,\n"
        "                    'stopped_early': True,\n"
        "                    'stop_reason': 'graphene_negative_evidence',\n"
        "                    'stop_detail': dmw_block_reason,\n"
        "                }\n"
        "                self._last_action_result = dict(compact_payload)\n"
        "                return {\n"
        "                    'action_result': compact_payload,\n"
        "                    'state': _serialized_runtime_state(last_action_result=compact_payload),\n"
        "                }\n"
        "            raw_payload = self._step_env_callback({\"actions\": normalized_actions})\n"
        "            dmw_after_frame, _ = load_runtime_state(state_path)\n"
        "            if not isinstance(raw_payload, dict):\n"
    )
    text = replace_once(text, old_step_block, new_step_block, "transition-capture-and-dead-action-block")
    old_normal_block = (
        "            if compact_payload.get(\"executed\") and _terminal_action_reason(compact_payload):\n"
        "                terminal_action_result = compact_payload\n"
        "            self._last_action_result = dict(compact_payload)\n"
        "            return {\n"
        "                \"action_result\": compact_payload,\n"
    )
    new_normal_block = (
        "            if compact_payload.get(\"executed\") and _terminal_action_reason(compact_payload):\n"
        "                terminal_action_result = compact_payload\n"
        "            self._last_action_result = dict(compact_payload)\n"
        "            if (\n"
        "                self._graphene_dmw is not None\n"
        "                and compact_payload.get('executed')\n"
        "                and dmw_before_frame is not None\n"
        "                and dmw_after_frame is not None\n"
        "            ):\n"
        "                self._graphene_dmw.record_transition(\n"
        "                    turn=int(compact_payload.get('action_num') or 0),\n"
        "                    action=dmw_action_label,\n"
        "                    before_grid=dmw_before_frame.grid,\n"
        "                    after_grid=dmw_after_frame.grid,\n"
        "                    level_before=dmw_before_frame.level,\n"
        "                    level_after=dmw_after_frame.level,\n"
        "                )\n"
        "            return {\n"
        "                \"action_result\": compact_payload,\n"
    )
    return replace_once(text, old_normal_block, new_normal_block, "transition-record")


def _patch_solver(original: str) -> str:
    text = replace_once(
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
    new_property = (
        "    @property\n"
        "    def kaggle_setup_commands(self) -> list[str]:\n"
        "        mode = str(self.graphene_dmw_mode or 'off').strip().lower()\n"
        "        if mode not in {'off', 'evidence', 'dialectic'}:\n"
        "            raise ValueError(f'Unsupported GRAPHENE_DMW_MODE: {mode}')\n"
        "        commands = []\n"
        "        if self.kaggle_enable_vllm:\n"
        "            commands.append(duck_kaggle_setup_command(self._kaggle_vllm_config()))\n"
        "        persist = (\n"
        "            '\"$PYTHON\" -c \'import json,os; p=os.environ[\"TAAF_KAGGLE_SETUP_ENV\"]; ' \n"
        "            'd=json.load(open(p)); d[\"GRAPHENE_DMW_MODE\"]=' + repr(mode) + '; ' \n"
        "            'open(p,\"w\").write(json.dumps(d,sort_keys=True))\''\n"
        "        )\n"
        "        commands.append(persist)\n"
        "        return commands\n"
    )
    return replace_once(text, old_property, new_property, "solver-kaggle-mode-persistence")


def patch_duck(duck_root: Path, bridge_source: Path) -> dict[str, str]:
    tool_agent = duck_root / "ARC3-Inference/inference/agent/tool_agent.py"
    solver = duck_root / "ARC3-Inference/inference/framework/solver.py"
    if not tool_agent.exists() or not solver.exists():
        raise FileNotFoundError(f"Duck source missing: {tool_agent} / {solver}")
    original_agent = tool_agent.read_text(encoding="utf-8")
    original_solver = solver.read_text(encoding="utf-8")
    patched_agent = _patch_tool_agent(original_agent)
    patched_solver = _patch_solver(original_solver)

    bridge_target = duck_root / "ARC3-Inference/inference/agent/graphene_dmw_bridge.py"
    shutil.copy2(bridge_source, bridge_target)
    tool_agent.write_text(patched_agent, encoding="utf-8")
    solver.write_text(patched_solver, encoding="utf-8")
    return {
        "upstream_commit": UPSTREAM_COMMIT,
        "tool_agent_original_sha256": hashlib.sha256(original_agent.encode()).hexdigest(),
        "tool_agent_patched_sha256": hashlib.sha256(patched_agent.encode()).hexdigest(),
        "solver_original_sha256": hashlib.sha256(original_solver.encode()).hexdigest(),
        "solver_patched_sha256": hashlib.sha256(patched_solver.encode()).hexdigest(),
        "bridge_sha256": hashlib.sha256(bridge_source.read_bytes()).hexdigest(),
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("duck_root")
    parser.add_argument("--bridge", default="arc_competition/graphene_dmw_duck_bridge.py")
    parser.add_argument("--manifest-out")
    args = parser.parse_args()
    manifest = patch_duck(Path(args.duck_root), Path(args.bridge))
    rendered = __import__("json").dumps(manifest, indent=2, sort_keys=True) + "\n"
    if args.manifest_out:
        Path(args.manifest_out).write_text(rendered, encoding="utf-8")
    print(rendered, end="")


if __name__ == "__main__":
    main()
