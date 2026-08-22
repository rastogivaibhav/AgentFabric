#!/usr/bin/env python3
from __future__ import annotations

"""Pinned-commit patch wrapper that disambiguates Duck's constructor-state anchor.

The base patcher intentionally fails unless anchors are unique. On the pinned Duck
commit the constructor assignment text appears twice because the 8-space substring
also matches the later 12-space session-reset assignment. For this one known anchor
we require exactly two textual matches, patch only the first occurrence, then allow
the base patcher's existing session-init anchor to consume the remaining occurrence.
All other anchors retain strict replace-once behavior.
"""

import arc_competition.apply_duck_graphene_patch as base

_original_replace_once = base.replace_once


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if label != "constructor-state":
        return _original_replace_once(text, old, new, label)
    count = text.count(old)
    if count != 2:
        raise RuntimeError(
            f"Duck patch anchor {label!r} expected exactly two textual matches on pinned commit, found {count}"
        )
    patched = text.replace(old, new, 1)
    # Fail closed: after constructor patch, exactly one nested session-init occurrence must remain.
    if patched.count(old) != 1:
        raise RuntimeError(
            f"Duck patch anchor {label!r} did not leave exactly one session-reset match"
        )
    return patched


base.replace_once = replace_once

if __name__ == "__main__":
    base.main()
