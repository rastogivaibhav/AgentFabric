import importlib.util
import json
import os
import pathlib
import statistics
import sys
import traceback
from importlib.metadata import version as package_version

ROOT = pathlib.Path(__file__).resolve().parents[1]
OUT = ROOT / "farmmind_render" / "out"
OUT.mkdir(parents=True, exist_ok=True)


def write_report(report):
    payload = json.dumps(report, indent=2, sort_keys=True)
    (OUT / "result.json").write_text(payload, encoding="utf-8")
    (OUT / "index.html").write_text(
        "<!doctype html><meta charset='utf-8'><title>FarmMind Kaggriculture Qualification</title>"
        "<pre>" + payload.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;") + "</pre>",
        encoding="utf-8",
    )
    print(payload, flush=True)


def load_submission():
    path = ROOT / "submission.py"
    spec = importlib.util.spec_from_file_location("farmmind_submission", path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"Unable to load submission from {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def bank(env, seat):
    # Prefer the actual terminal farm bank. Kaggriculture ranking rewards can differ
    # from raw bank depending on execution context, while final farm.money is the
    # competition's economic outcome we are targeting.
    for states in reversed(env.steps):
        try:
            player_id = int(states[seat].observation.player)
            return float(states[seat].observation.farms[player_id].money)
        except Exception:
            continue
    raise RuntimeError(f"Unable to resolve terminal bank for seat {seat}")


report = {"status": "starting"}
write_report(report)
exit_code = 0

try:
    from kaggle_environments import make
    submission = load_submission()

    candidate = submission.agent
    parent = submission._FM21_PARENT_AGENT
    seeds = int(os.getenv("FARMMIND_SEEDS", "3"))
    seed_base = int(os.getenv("FARMMIND_SEED_BASE", "916001"))
    rows = []

    for offset in range(seeds):
        seed = seed_base + offset
        for candidate_seat in (0, 1):
            pair = [candidate, parent] if candidate_seat == 0 else [parent, candidate]
            env = make(
                "kaggriculture",
                configuration={"episodeSteps": 720, "seed": seed},
                debug=False,
            )
            env.run(pair)
            final = env.steps[-1]
            statuses = [str(final[0].status), str(final[1].status)]
            if statuses != ["DONE", "DONE"]:
                raise RuntimeError(f"Non-DONE status seed={seed} seat={candidate_seat}: {statuses}")
            candidate_bank = bank(env, candidate_seat)
            parent_bank = bank(env, 1 - candidate_seat)
            margin = candidate_bank - parent_bank
            row = {
                "seed": seed,
                "candidate_seat": candidate_seat,
                "candidate_bank": candidate_bank,
                "parent_bank": parent_bank,
                "margin": margin,
                "result": "win" if margin > 0 else "loss" if margin < 0 else "tie",
                "turns": len(env.steps),
            }
            rows.append(row)
            print("FARMMIND_GAME=" + json.dumps(row, sort_keys=True), flush=True)

    wins = sum(r["margin"] > 0 for r in rows)
    losses = sum(r["margin"] < 0 for r in rows)
    ties = len(rows) - wins - losses
    margins = [r["margin"] for r in rows]
    candidate_banks = [r["candidate_bank"] for r in rows]
    parent_banks = [r["parent_bank"] for r in rows]
    win_rate = wins / len(rows) if rows else 0.0
    mean_margin = statistics.fmean(margins) if margins else 0.0

    report = {
        "status": "complete",
        "environment": "kaggriculture",
        "kaggle_environments_version": package_version("kaggle-environments"),
        "seeds": seeds,
        "seed_base": seed_base,
        "games": len(rows),
        "wins": wins,
        "losses": losses,
        "ties": ties,
        "win_rate": win_rate,
        "mean_margin": mean_margin,
        "median_margin": statistics.median(margins) if margins else 0.0,
        "mean_candidate_bank": statistics.fmean(candidate_banks) if candidate_banks else 0.0,
        "mean_parent_bank": statistics.fmean(parent_banks) if parent_banks else 0.0,
        "max_candidate_bank": max(candidate_banks) if candidate_banks else None,
        "max_parent_bank": max(parent_banks) if parent_banks else None,
        "min_candidate_bank": min(candidate_banks) if candidate_banks else None,
        "promote": bool(win_rate >= 0.55 and mean_margin > 0),
        "promotion_rule": "win_rate >= 0.55 and mean_margin > 0",
        "rows": rows,
    }
except Exception as exc:
    exit_code = 1
    report = {
        "status": "error",
        "error": repr(exc),
        "traceback": traceback.format_exc(),
    }

write_report(report)
sys.exit(exit_code)
