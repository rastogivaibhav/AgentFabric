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


def patch_duck(duck_root: Path, bridge_source: Path) -> dict[str, str]:
    tool_agent = duck_root / "ARC3-Inference/inference/agent/tool_agent.py"
    if not tool_agent.exists():
        raise FileNotFoundError(tool_agent)
    original = tool_agent.read_text(encoding="utf-8")
    original_sha = hashlib.sha256(original.encode()).hexdigest()

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
    text = replace_once(
        text,
        "            raw_payload = self._step_env_callback({\"actions\": normalized_actions})\n"
        "            if not isinstance(raw_payload, dict):\n",
        "            dmw_before_frame, _ = load_runtime_state(state_path)\n"
        "            raw_payload = self._step_env_callback({\"actions\": normalized_actions})\n"
        "            dmw_after_frame, _ = load_runtime_state(state_path)\n"
        "            if not isinstance(raw_payload, dict):\n",
        "transition-capture",
    )
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
        "                executed = compact_payload.get('executed_actions') or [compact_payload.get('action_display') or 'UNKNOWN']\n"
        "                action_label = ' -> '.join(str(item) for item in executed if str(item).strip())\n"
        "                self._graphene_dmw.record_transition(\n"
        "                    turn=int(compact_payload.get('action_num') or 0),\n"
        "                    action=action_label or 'UNKNOWN',\n"
        "                    before_grid=dmw_before_frame.grid,\n"
        "                    after_grid=dmw_after_frame.grid,\n"
        "                    level_before=dmw_before_frame.level,\n"
        "                    level_after=dmw_after_frame.level,\n"
        "                )\n"
        "            return {\n"
        "                \"action_result\": compact_payload,\n"
    )
    text = replace_once(text, old_normal_block, new_normal_block, "transition-record")

    bridge_target = duck_root / "ARC3-Inference/inference/agent/graphene_dmw_bridge.py"
    shutil.copy2(bridge_source, bridge_target)
    tool_agent.write_text(text, encoding="utf-8")
    return {
        "upstream_commit": UPSTREAM_COMMIT,
        "tool_agent_original_sha256": original_sha,
        "tool_agent_patched_sha256": hashlib.sha256(text.encode()).hexdigest(),
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
