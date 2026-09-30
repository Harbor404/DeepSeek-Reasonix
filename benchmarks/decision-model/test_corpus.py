import tempfile
import unittest
from pathlib import Path

from corpus import export_corpora, metrics


class CorpusTests(unittest.TestCase):
    def test_failed_generation_still_counts_as_attempted_and_timed(self):
        result = metrics([{"label": "a", "prediction": None, "latency_ms": 200}])
        self.assertEqual(result["attempted"], 1)
        self.assertEqual(result["coverage"], 0)
        self.assertEqual(result["latency_ms_p50"], 200)

    def test_skips_are_in_accuracy_denominator(self):
        result = metrics([
            {"label": "a", "prediction": "a", "latency_ms": 10},
            {"label": "a", "prediction": "b", "latency_ms": 20},
            {"label": "b", "prediction": None},
        ])
        self.assertAlmostEqual(result["accuracy"], 1 / 3)
        self.assertEqual(result["balanced_accuracy"], 0.25)
        self.assertAlmostEqual(result["coverage"], 2 / 3)
        self.assertEqual(result["per_class"]["b"]["recall"], 0)

    def test_cross_split_duplicates_are_rejected(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            for corpus, prompt in (("train", "Fix it"), ("e2e", "Fix  it")):
                task = root / f"benchmarks/{corpus}/tasks/example"
                task.mkdir(parents=True)
                (task / "task.toml").write_text(
                    f'prompt = "{prompt}"\nclass = "atomic-bugfix"\n', encoding="utf-8")
            with self.assertRaises(ValueError):
                export_corpora(root, root / "out")


if __name__ == "__main__":
    unittest.main()
