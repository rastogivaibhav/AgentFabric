#!/usr/bin/env python3
from __future__ import annotations

"""Pinned-commit patch wrapper for Duck + GrapheneDMW integration.

The base patcher now uses a context-specific constructor anchor. This wrapper only
retains the Kaggle solver quoting fix required by the pinned Duck commit.
"""

import sys
from pathlib import Path

try:
    import apply_duck_graphene_patch as base
except ModuleNotFoundError:
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    import apply_duck_graphene_patch as base

_original_replace_once = base.replace_once


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


base._patch_solver = patch_solver

if __name__ == "__main__":
    base.main()
