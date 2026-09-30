import hashlib
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from corpus import CLASSES
from head_model import HeadScorer
from model_config import FEATURE_SCHEMA, MODEL_ID, REVISION


class HeadContractTests(unittest.TestCase):
    def test_changed_taxonomy_is_rejected_before_loading_backbone(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder)
            (path / "metadata.json").write_text(json.dumps({
                "labels": ["wrong-contract"], "model": MODEL_ID, "feature_schema": FEATURE_SCHEMA,
            }), encoding="utf-8")
            with patch("head_model.FrozenEncoder") as encoder:
                with self.assertRaises(ValueError):
                    HeadScorer(path)
                encoder.assert_not_called()

    def test_tampered_weights_are_rejected_before_loading_backbone(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder)
            expected = hashlib.sha256(b"original").hexdigest()
            (path / "head.safetensors").write_bytes(b"changed")
            (path / "metadata.json").write_text(json.dumps({
                "labels": list(CLASSES), "model": MODEL_ID, "feature_schema": FEATURE_SCHEMA,
                "head_sha256": expected, "revision": REVISION,
            }), encoding="utf-8")
            with patch("head_model.FrozenEncoder") as encoder:
                with self.assertRaises(ValueError):
                    HeadScorer(path)
                encoder.assert_not_called()


if __name__ == "__main__":
    unittest.main()
