#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
from pathlib import Path

FROZEN_SHA256 = "63f205b3dbbad0d580dc8a262edbf8dd432c3ce6df173e0f888ac2188eeb78d1"

INJECT = r'''

def a14_prior_bundle(graphene_helper: str) -> dict[str, Any]:
    prior_db = Path(os.environ["A14_PRIOR_DB"])
    retrieval_log = Path(os.environ["A14_PRIOR_RETRIEVAL_LOG"])
    expected = int(os.environ.get("A14_PRIOR_COUNT", "8"))
    prior_value = 0.125
    qvec = ",".join(f"{prior_value:.8g}" for _ in range(GRAPHENE_DIM))
    proc = subprocess.run(
        [graphene_helper, "query", str(prior_db), str(GRAPHENE_DIM), qvec, str(expected)],
        check=True, capture_output=True, text=True, timeout=30,
    )
    data = json.loads(proc.stdout)
    items = []
    for hit in data:
        try:
            obj = json.loads(hit.get("content", "{}"))
        except Exception:
            continue
        if obj.get("kind") != "universal_prior":
            continue
        items.append({
            "prior_id": obj.get("prior_id"),
            "category": obj.get("category"),
            "instruction": obj.get("instruction"),
            "strength": obj.get("strength"),
        })
    items.sort(key=lambda x: str(x.get("prior_id")))
    if len(items) != expected:
        raise RuntimeError(f"A1.4 prior retrieval incomplete: expected={expected} got={len(items)}")
    retrieval_log.parent.mkdir(parents=True, exist_ok=True)
    with retrieval_log.open("a", encoding="utf-8") as f:
        f.write(json.dumps({
            "prior_db": str(prior_db),
            "prior_hits": len(items),
            "prior_ids": [x.get("prior_id") for x in items],
            "source": "graphenedb_vector_search",
        }, sort_keys=True) + "\n")
    return {
        "id": "a14-universal-prior-bundle",
        "score": 1.0,
        "episode": {
            "kind": "universal_prior_bundle",
            "source": "GrapheneDB",
            "instruction": "These are generic transferable priors, not facts about this game. Verify them from current-game evidence.",
            "items": items,
        },
    }
'''


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--frozen", required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()

    frozen = Path(args.frozen)
    output = Path(args.output)
    raw = frozen.read_bytes()
    digest = hashlib.sha256(raw).hexdigest()
    if digest != FROZEN_SHA256:
        raise SystemExit(f"frozen agent hash mismatch: {digest}")
    src = raw.decode("utf-8")

    marker = "\ndef play_game("
    if src.count(marker) != 1:
        raise SystemExit("unexpected play_game marker count")
    src = src.replace(marker, INJECT + marker, 1)

    old = "            prompt = build_prompt(game_id, obs, available, memory, grid, episodes)\n"
    new = (
        "            prior_bundle = a14_prior_bundle(graphene_helper)\n"
        "            prompt = build_prompt(game_id, obs, available, memory, grid, episodes + [prior_bundle])\n"
    )
    if src.count(old) != 1:
        raise SystemExit("unexpected build_prompt call count")
    src = src.replace(old, new, 1)

    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(src, encoding="utf-8")
    patched = hashlib.sha256(output.read_bytes()).hexdigest()
    print(f"a14_treatment_build=PASS frozen_sha256={digest} treatment_sha256={patched} output={output}")


if __name__ == "__main__":
    main()
