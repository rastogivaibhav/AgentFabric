#!/usr/bin/env python3
"""Materialize A1.4 generic prior as deterministic structured seed records.

This file does not contain target-game knowledge. It converts the tiny prior JSON
into stable records that the A1.4 runner can insert into GrapheneDB before play.
"""
from __future__ import annotations

import hashlib
import json
import sys
from pathlib import Path


def stable_vector(text: str, dim: int = 64):
    out = []
    counter = 0
    while len(out) < dim:
        digest = hashlib.sha256(f"{counter}:{text}".encode()).digest()
        for b in digest:
            out.append((b / 127.5) - 1.0)
            if len(out) == dim:
                break
        counter += 1
    return out


def main():
    src = Path(sys.argv[1] if len(sys.argv) > 1 else "pi45_a1/a14_prior.json")
    dst = Path(sys.argv[2] if len(sys.argv) > 2 else "/tmp/a14_seed_records.jsonl")
    prior = json.loads(src.read_text())
    rows = []
    for i, p in enumerate(prior["principles"]):
        payload = {
            "seed": True,
            "prior_name": prior["prior_name"],
            "prior_id": p["id"],
            "category": p["category"],
            "text": p["text"],
            "target_game_specific": False,
            "ordinal": i,
        }
        rows.append({
            "id": f"a14-prior-{i:03d}-{p['id']}",
            "text": p["text"],
            "metadata": payload,
            "vector": stable_vector(f"{p['category']}::{p['text']}")
        })
    dst.parent.mkdir(parents=True, exist_ok=True)
    dst.write_text("".join(json.dumps(r, sort_keys=True) + "\n" for r in rows))
    digest = hashlib.sha256(dst.read_bytes()).hexdigest()
    print(f"a14_seed_records={len(rows)} sha256={digest} path={dst}")


if __name__ == "__main__":
    main()
