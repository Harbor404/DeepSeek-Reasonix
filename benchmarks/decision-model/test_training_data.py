import json
import tempfile
import unittest
from pathlib import Path

from corpus import CLASSES
from training_data import audit_splits, build_bundle


class TrainingDataTests(unittest.TestCase):
    def test_actual_bundle_keeps_translations_and_wrappers_together(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            build_bundle(root, root / "out")
            seen = {}
            for split in ("train", "validation", "test"):
                rows = [json.loads(line) for line in (root / f"out/{split}.jsonl").read_text(
                    encoding="utf-8").splitlines()]
                self.assertEqual({row["label"] for row in rows}, set(CLASSES))
                for row in rows:
                    if row["family"] in seen:
                        self.assertEqual(seen[row["family"]], split)
                    seen[row["family"]] = split
                for family in {row["family"] for row in rows}:
                    variants = [row for row in rows if row["family"] == family]
                    self.assertEqual(len(variants), 6)
                    self.assertEqual({row["language"] for row in variants}, {"en", "zh"})

    def test_translation_in_another_split_is_rejected(self):
        with self.assertRaises(ValueError):
            audit_splits({
                "train": [{"family": "same-scenario", "prompt": "Fix the sign comparison"}],
                "test": [{"family": "same-scenario", "prompt": "修正符号比较"}],
            })


if __name__ == "__main__":
    unittest.main()
