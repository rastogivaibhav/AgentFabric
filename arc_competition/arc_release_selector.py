#!/usr/bin/env python3
"""ARC-AGI-3 release selector: never upgrade on synthetic or incomplete evidence.

This verifies the off/evidence Kaggle dry-run identity and paired *real-game*
evaluation results. It intentionally cannot attest that Kaggle ran or scored a
submission; that requires a Kaggle-generated version and leaderboard entry.
"""
import argparse
import json
import math
from pathlib import Path

MODEL = 'vrfai/Qwen3.6-27B-FP8'


def read_identity(path, expected_mode):
    fields = {}
    for line in Path(path).read_text(encoding='utf-8').splitlines():
        if '=' in line:
            k, v = line.split('=', 1)
            fields[k.strip()] = v.strip()
    expected = {'mode': expected_mode, 'model': MODEL,
                'internet': 'false', 'kaggle_dry_run': 'true'}
    errors = [f'{k}: expected {v!r}, got {fields.get(k)!r}'
              for k, v in expected.items() if fields.get(k) != v]
    if errors:
        raise ValueError('invalid packaging identity: ' + '; '.join(errors))
    return fields


def read_scores(path):
    data = json.loads(Path(path).read_text(encoding='utf-8'))
    if data.get('competition_valid_model') is False:
        raise ValueError('noncompetition model: diagnostic scores are not release evidence')
    if not data.get('real_game_execution', False):
        raise ValueError('missing explicit real_game_execution=true attestation')
    if data.get('model') != MODEL:
        raise ValueError('evaluation must attest pinned competition model')
    games = data.get('games')
    if not isinstance(games, dict) or not games:
        raise ValueError('missing game-level evaluation results')
    scores = {}
    for game, row in games.items():
        if not isinstance(row, dict) or type(row.get('score')) not in (int, float):
            raise ValueError(f'invalid game score for {game}')
        score = float(row['score'])
        if not math.isfinite(score) or not (0 <= score <= 100):
            raise ValueError(f'out-of-range score for {game}')
        scores[str(game)] = score
    return scores


def select(off_identity, evidence_identity, off_eval=None, evidence_eval=None):
    """Fail closed to OFF unless a matched, offline, competition-model ablation wins."""
    out = {'selected': 'off', 'decision': 'BASELINE_ONLY',
           'kaggle_submission_verified': False, 'reasons': [], 'scores': {}}
    try:
        read_identity(off_identity, 'off')
    except (OSError, ValueError) as exc:
        out['decision'] = 'BLOCKED'
        out['reasons'].append(f'off package: {exc}')
        return out
    try:
        read_identity(evidence_identity, 'evidence')
    except (OSError, ValueError) as exc:
        out['reasons'].append(f'evidence package: {exc}')
        return out
    if not off_eval or not evidence_eval:
        out['reasons'].append('no paired real-game competition-model evaluations')
        return out
    try:
        off = read_scores(off_eval)
        evidence = read_scores(evidence_eval)
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        out['reasons'].append(f'unusable evaluation: {exc}')
        return out
    if off.keys() != evidence.keys() or len(off) < 3:
        out['reasons'].append('requires at least three identical game IDs in both arms')
        return out
    diff = {g: evidence[g] - off[g] for g in sorted(off)}
    wins = sum(d > 0 for d in diff.values())
    mean_diff = sum(diff.values()) / len(diff)
    out['scores'] = {'games': len(diff), 'wins': wins, 'mean_delta': mean_diff,
                     'per_game_delta': diff}
    if mean_diff > 0 and wins >= 2:
        out['selected'] = 'evidence'
        out['decision'] = 'EVIDENCE_READY_FOR_KAGGLE_VALIDATION'
        out['reasons'].append('matched local games favour evidence; Kaggle score still unverified')
    else:
        out['reasons'].append('evidence did not beat control on the preregistered gate')
    return out


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--off-package-identity', required=True)
    parser.add_argument('--evidence-package-identity', required=True)
    parser.add_argument('--off-eval')
    parser.add_argument('--evidence-eval')
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    report = select(args.off_package_identity, args.evidence_package_identity,
                    args.off_eval, args.evidence_eval)
    dest = Path(args.output)
    dest.parent.mkdir(parents=True, exist_ok=True)
    dest.write_text(json.dumps(report, indent=2, sort_keys=True) + '\n', encoding='utf-8')
    print(json.dumps(report, indent=2, sort_keys=True))
    raise SystemExit(2 if report['decision'] == 'BLOCKED' else 0)


if __name__ == '__main__':
    main()
