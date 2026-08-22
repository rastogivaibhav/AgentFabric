#!/usr/bin/env python3
from __future__ import annotations

"""Pinned-commit patch wrapper that disambiguates Duck's constructor-state anchor."""

import sys
from pathlib import Path

try:
    import apply_duck_graphene_patch as base
except ModuleNotFoundError:
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    import apply_duck_graphene_patch as base

_original_replace_once = base.replace_once
_SESSION_RESET = "            self._summarized_knowledge = _empty_world_model()\n"


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if label != "constructor-state":
        return _original_replace_once(text, old, new, label)
    count = text.count(old)
    nested_count = text.count(_SESSION_RESET)
    if count != 2 or nested_count != 1:
        raise RuntimeError(
            f"Duck patch anchor {label!r} expected two textual matches with one nested session reset; "
            f"found total={count}, nested={nested_count}"
        )
    patched = text.replace(old, new, 1)
    if patched.count(_SESSION_RESET) != 1:
        raise RuntimeError(
            f"Duck patch anchor {label!r} disturbed the unique nested session-reset anchor"
        )
    return patched


base.replace_once = replace_once

if __name__ == "__main__":
    base.main()
