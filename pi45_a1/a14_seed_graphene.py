#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import math
import shutil
import subprocess
from pathlib import Path

DIM = 64
PRIOR_VECTOR_VALUE = 1.0 / math.sqrt(DIM)


def vector_csv() -> str:
    return ",".join(f"{PRIOR_VECTOR_VALUE:.8g}" for _ in range(DIM))


def remove_existing(path: Path) -> None:
    if path.is_dir():
        shutil.rmtree(path)
    elif path.exists():
        path.unlink()


def helper_stats(helper: str, db: Path) -> dict:
    proc = subprocess.run(
        [helper, "stats", str(db), str(DIM)],
        check=True,
        capture_output=True,
        text=True,
        timeout=30,
    )
    return json.loads(proc.stdout)


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--prior", required=True)
    ap.add_argument("--helper", required=True)
    ap.add_argument("--db", required=True)
    ap.add_argument("--manifest", required=True)
    args = ap.parse_args()

    prior_path = Path(args.prior)
    db_path = Path(args.db)
    manifest_path = Path(args.manifest)
    prior = json.loads(prior_path.read_text(encoding="utf-8"))
    memories = list(prior.get("memories") or [])
    if len(memories) != 8:
        raise SystemExit(f"expected exactly 8 minimal prior memories, found {len(memories)}")

    forbidden = ("ls20", "ft09", "bp35")
    raw = prior_path.read_text(encoding="utf-8").lower()
    for token in forbidden:
        if token in raw:
            raise SystemExit(f"target-game contamination token present in prior: {token}")

    remove_existing(db_path)
    db_path.parent.mkdir(parents=True, exist_ok=True)
    manifest_path.parent.mkdir(parents=True, exist_ok=True)

    node_ids = []
    vec = vector_csv()
    for ordinal, memory in enumerate(memories):
        content_obj = {
            "kind": "universal_prior",
            "prior_name": prior["prior_name"],
            "prior_id": str(memory["id"]),
            "category": str(memory["category"]),
            "instruction": str(memory["instruction"]),
            "strength": "weak_prior_verify_in_game",
            "target_game_specific": False,
            "ordinal": ordinal,
        }
        content = json.dumps(content_obj, sort_keys=True, separators=(",", ":"))
        signature = int(hashlib.sha256(content.encode("utf-8")).hexdigest()[:16], 16)
        proc = subprocess.run(
            [args.helper, "put", str(db_path), str(DIM), content, vec, str(signature)],
            check=True,
            capture_output=True,
            text=True,
            timeout=30,
        )
        node_ids.append(int(proc.stdout.strip()))

    stats = helper_stats(args.helper, db_path)
    assert stats.get("valid") is True, stats
    assert stats.get("edges") == 0, stats
    assert stats.get("nodes") == len(memories), stats

    manifest = {
        "prior_name": prior["prior_name"],
        "prior_sha256": hashlib.sha256(prior_path.read_bytes()).hexdigest(),
        "prior_memory_count": len(memories),
        "dimension": DIM,
        "vector_policy": "fixed_normalized_prior_anchor",
        "vector_value": PRIOR_VECTOR_VALUE,
        "node_ids": node_ids,
        "db_path": str(db_path),
        "stats": stats,
        "target_game_specific": False,
    }
    manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(
        f"a14_seed_graphenedb=PASS prior={prior['prior_name']} nodes={len(memories)} "
        f"sha256={manifest['prior_sha256']} db={db_path}",
        flush=True,
    )


if __name__ == "__main__":
    main()
