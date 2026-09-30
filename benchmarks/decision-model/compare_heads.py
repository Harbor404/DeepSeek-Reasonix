import argparse
import hashlib
import json
from pathlib import Path

from corpus import CLASSES
from encoder import FrozenEncoder, default_cache
from external_dataset import load_bundle
from model_config import FEATURE_SCHEMA, REVISION
from train_head import score_rows


def main():
    from safetensors.torch import load_file

    parser = argparse.ArgumentParser()
    parser.add_argument("--data", type=Path, required=True)
    parser.add_argument("--checkpoint", type=Path, action="append", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    manifest, splits = load_bundle(args.data)
    encoder = FrozenEncoder(default_cache())
    features, _ = encoder.encode_rows(splits["test"])
    comparisons = {}
    for folder in args.checkpoint:
        metadata = json.loads((folder / "metadata.json").read_text(encoding="utf-8"))
        weights = folder / "head.safetensors"
        digest = hashlib.sha256(weights.read_bytes()).hexdigest()
        if (digest != metadata["head_sha256"] or metadata["revision"] != REVISION
                or metadata["labels"] != list(CLASSES) or metadata["feature_schema"] != FEATURE_SCHEMA
                or metadata["max_tokens"] != encoder.max_tokens):
            raise ValueError("Checkpoint identity differs from comparison encoder")
        report, predictions = score_rows(encoder.torch, load_file(str(weights)), features, splits["test"])
        comparisons[folder.name] = {"head_sha256": digest, "metrics": report,
                                    "predictions": [{"url": r["source_url"], "label": r["label"],
                                                     "prediction": r["prediction"]} for r in predictions]}
    output = {"model_revision": REVISION, "test_sha256": manifest["splits"]["test"]["sha256"],
              "metric_contract": manifest["metric_contract"],
              "limitations": manifest["limitations"], "comparisons": comparisons,
              "timing": "Shared cached frozen features; no per-request speed measurement"}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(output, indent=2), encoding="utf-8")
    print(json.dumps({key: {"correct": value["metrics"]["correct"],
                           "count": value["metrics"]["count"],
                           "accuracy": value["metrics"]["accuracy"],
                           "balanced_accuracy": value["metrics"]["balanced_accuracy"],
                           "majority": value["metrics"]["majority_baseline_accuracy"]}
                      for key, value in comparisons.items()}, indent=2))


if __name__ == "__main__":
    main()
