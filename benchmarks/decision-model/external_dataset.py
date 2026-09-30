import hashlib
import json

from corpus import CLASSES, fingerprint
from training_data import audit_splits


def load_bundle(directory):
    manifest = json.loads((directory / "manifest.json").read_text(encoding="utf-8"))
    if manifest["labels"] != list(CLASSES):
        raise ValueError("Dataset taxonomy differs from classifier")
    splits, repositories = {}, {}
    for split in ("train", "validation", "test"):
        path = directory / f"{split}.jsonl"
        info = manifest["splits"][split]
        if hashlib.sha256(path.read_bytes()).hexdigest() != info["sha256"]:
            raise ValueError("Dataset hash mismatch")
        rows = [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines()]
        if not rows or len(rows) != info["count"]:
            raise ValueError("Dataset count mismatch")
        for row in rows:
            if row["label"] not in CLASSES or fingerprint(row["prompt"]) != row["sha256"]:
                raise ValueError("Invalid dataset label or prompt hash")
            repository = row.get("repository")
            if repository:
                if repository in repositories and repositories[repository] != split:
                    raise ValueError("GitHub repository crosses splits")
                repositories[repository] = split
        splits[split] = rows
    audit_splits(splits)
    return manifest, splits
