import hashlib
import json
import tempfile
import unittest
from pathlib import Path

from corpus import CLASSES, fingerprint
from external_dataset import load_bundle


class ExternalDatasetTests(unittest.TestCase):
    def bundle(self, directory, same_repository=False):
        manifest = {"labels": list(CLASSES), "splits": {}}
        for split in ("train", "validation", "test"):
            prompt = "Request " + split
            row = {"prompt": prompt, "sha256": fingerprint(prompt), "family": split,
                   "label": "atomic-bugfix", "repository": "same" if same_repository else split}
            path = directory / (split + ".jsonl")
            path.write_text(json.dumps(row) + "\n", encoding="utf-8")
            manifest["splits"][split] = {"count": 1, "sha256": hashlib.sha256(path.read_bytes()).hexdigest()}
        (directory / "manifest.json").write_text(json.dumps(manifest), encoding="utf-8")

    def test_repository_cannot_cross_splits(self):
        with tempfile.TemporaryDirectory() as folder:
            directory = Path(folder)
            self.bundle(directory, same_repository=True)
            with self.assertRaisesRegex(ValueError, "repository crosses"):
                load_bundle(directory)

    def test_modified_dataset_rejected(self):
        with tempfile.TemporaryDirectory() as folder:
            directory = Path(folder)
            self.bundle(directory)
            load_bundle(directory)
            with (directory / "test.jsonl").open("a", encoding="utf-8") as handle:
                handle.write("{}\n")
            with self.assertRaisesRegex(ValueError, "hash mismatch"):
                load_bundle(directory)


if __name__ == "__main__":
    unittest.main()
