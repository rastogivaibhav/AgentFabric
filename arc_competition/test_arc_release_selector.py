"""Safety and decision tests for ARC-AGI-3 release gate (stdlib-only)."""
import json
import tempfile
import unittest
from pathlib import Path
from arc_release_selector import MODEL, read_scores, select


class TestReleaseSelector(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        for mode in ('off', 'evidence'):
            (self.root / f'{mode}.txt').write_text(
                f'mode={mode}\nmodel={MODEL}\ninternet=false\nkaggle_dry_run=true\n')

    def path(self, mode):
        return self.root / f'{mode}.txt'

    def evaluation(self, mode, scores, **extra):
        x = dict(model=MODEL, real_game_execution=True,
                 games={g: {'score': v} for g, v in scores.items()})
        x.update(extra)
        p = self.root / f'{mode}.json'
        p.write_text(json.dumps(x))
        return p

    def test_missing_scores_keeps_baseline(self):
        r = select(self.path('off'), self.path('evidence'))
        self.assertEqual(r['selected'], 'off')
        self.assertFalse(r['kaggle_submission_verified'])

    def test_matched_improvement_selects_evidence(self):
        a = self.evaluation('off', {'g1': 1, 'g2': 2, 'g3': 3})
        b = self.evaluation('evidence', {'g1': 6, 'g2': 5, 'g3': 1})
        r = select(self.path('off'), self.path('evidence'), a, b)
        self.assertEqual(r['selected'], 'evidence')
        self.assertFalse(r['kaggle_submission_verified'])

    def test_noncompetition_diagnostic_rejected(self):
        a = self.evaluation('off', {'g1': 1, 'g2': 2, 'g3': 3})
        b = self.evaluation('evidence', {'g1': 9, 'g2': 9, 'g3': 9},
                            competition_valid_model=False)
        r = select(self.path('off'), self.path('evidence'), a, b)
        self.assertEqual(r['selected'], 'off')

    def test_unmatched_game_set_rejected(self):
        a = self.evaluation('off', {'g1': 1, 'g2': 2, 'g3': 3})
        b = self.evaluation('evidence', {'g1': 9, 'g2': 9, 'g4': 9})
        self.assertEqual(select(self.path('off'), self.path('evidence'), a, b)['selected'], 'off')

    def test_invalid_offline_identity_blocks(self):
        self.path('off').write_text('mode=off\nmodel=fake\ninternet=true\n')
        r = select(self.path('off'), self.path('evidence'))
        self.assertEqual(r['decision'], 'BLOCKED')

    def test_nan_score_rejected(self):
        p = self.evaluation('off', {'g1': float('nan')})
        with self.assertRaises(ValueError):
            read_scores(p)


if __name__ == '__main__':
    unittest.main()
