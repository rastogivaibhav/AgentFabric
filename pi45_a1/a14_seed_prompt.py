#!/usr/bin/env python3
"""A1.4 prompt adapter: injects a tiny generic interactive-world prior into the
existing frozen A1 episodic agent without changing model weights or target-game
knowledge. The adapter is intentionally treatment-scoped and records provenance.
"""
from __future__ import annotations

import json
import os
import runpy
from pathlib import Path

PRIOR_PATH = Path(os.environ.get("A14_PRIOR_PATH", "pi45_a1/a14_prior.json"))
TARGET = Path(os.environ.get("A14_FROZEN_AGENT", "/tmp/pi45a1/arc_a1_graphene_episodic.py"))

prior = json.loads(PRIOR_PATH.read_text(encoding="utf-8"))
lines = [
    "MINIMAL INTERACTIVE WORLD PRIOR (generic; not game-specific):",
    "Use these as weak priors, not facts about this particular game.",
]
for p in prior["principles"]:
    lines.append(f"- [{p['category']}] {p['text']}")
seed_text = "\n".join(lines)

# The frozen agent reads PI45A1_SYSTEM_APPEND when constructing its system
# prompt in this execution harness. The source itself remains byte-identical.
os.environ["PI45A1_SYSTEM_APPEND"] = seed_text
os.environ["A14_PRIOR_NAME"] = prior["prior_name"]
os.environ["A14_PRIOR_NODE_COUNT"] = str(len(prior["principles"]))
print(f"a14_prior=ACTIVE name={prior['prior_name']} nodes={len(prior['principles'])}")
runpy.run_path(str(TARGET), run_name="__main__")
