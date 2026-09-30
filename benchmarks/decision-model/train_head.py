import argparse
import hashlib
import json
import time
from pathlib import Path

from corpus import CLASSES, metrics
from encoder import FrozenEncoder, default_cache
from model_config import FEATURE_SCHEMA, MODEL_ID, REVISION
from training_data import build_bundle


def read_rows(path):
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines()]


def fit_head(torch, train_features, train_labels, val_features, val_labels):
    torch.manual_seed(42)
    torch.set_num_threads(4)
    mean = train_features.mean(dim=0)
    std = train_features.std(dim=0, correction=0).clamp_min(0.05)
    train = (train_features - mean) / std
    validation = (val_features - mean) / std
    head = torch.nn.Linear(train.shape[1], len(CLASSES))
    torch.nn.init.zeros_(head.weight)
    torch.nn.init.zeros_(head.bias)
    counts = torch.bincount(train_labels, minlength=len(CLASSES)).float()
    if (counts == 0).any():
        raise ValueError("Training data lacks a task category")
    loss_fn = torch.nn.CrossEntropyLoss(weight=len(train_labels) / (len(CLASSES) * counts))
    optimizer = torch.optim.AdamW(head.parameters(), lr=0.003, weight_decay=0.1)
    best_loss, selected, selected_epoch, stale = float("inf"), None, None, 0
    history = []
    started = time.perf_counter()
    for epoch in range(1, 401):
        optimizer.zero_grad(set_to_none=True)
        loss = loss_fn(head(train), train_labels)
        loss.backward()
        optimizer.step()
        with torch.no_grad():
            val_logits = head(validation)
            val_loss = float(torch.nn.functional.cross_entropy(val_logits, val_labels))
            val_accuracy = float((val_logits.argmax(dim=1) == val_labels).float().mean())
        history.append({"epoch": epoch, "train_loss": float(loss.detach()),
                        "validation_loss": val_loss, "validation_accuracy": val_accuracy})
        if val_loss < best_loss - 1e-5:
            best_loss, selected_epoch, stale = val_loss, epoch, 0
            selected = {key: tensor.detach().clone() for key, tensor in head.state_dict().items()}
        else:
            stale += 1
        if epoch == 1 or epoch % 25 == 0:
            print(f"Epoch {epoch}: validation accuracy {val_accuracy:.3f}, loss {val_loss:.3f}", flush=True)
        if stale >= 40:
            break
    tensors = {**selected, "mean": mean, "std": std}
    return tensors, {"selected_epoch": selected_epoch, "epochs_run": len(history),
                     "validation_loss": best_loss, "head_training_seconds": time.perf_counter() - started,
                     "history": history}


def score_rows(torch, tensors, features, rows):
    normalized = (features - tensors["mean"]) / tensors["std"]
    with torch.no_grad():
        logits = normalized @ tensors["weight"].T + tensors["bias"]
        predicted = logits.argmax(dim=1).tolist()
    labels = list(CLASSES)
    outputs = [{**row, "prediction": labels[index]} for row, index in zip(rows, predicted)]
    report = metrics(outputs)
    report["languages"] = {language: metrics([r for r in outputs if r["language"] == language])
                           for language in sorted({r["language"] for r in rows})}
    for language_report in report["languages"].values():
        language_report["attempted"] = language_report["count"]
    family_results = {}
    for row in outputs:
        family_results.setdefault(row["family"], []).append(row["prediction"] == row["label"])
    report["families"] = {"count": len(family_results),
                           "all_variants_correct": sum(all(flags) for flags in family_results.values()),
                           "mean_family_accuracy": sum(sum(flags) / len(flags) for flags in family_results.values())
                           / len(family_results)}
    report["attempted"] = len(rows)
    report["latency_ms_p50"] = None
    report["latency_ms_p95"] = None
    return report, outputs


def main():
    from safetensors.torch import save_file

    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path)
    parser.add_argument("--data", type=Path, help="Audited external bundle with manifest and three JSONL splits")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[2]
    output = args.output or repo / "temp/decision-model/head-v1"
    if (output / "metadata.json").exists():
        parser.error("Checkpoint already exists; choose a new --output directory")
    output.mkdir(parents=True, exist_ok=True)
    data = args.data or output / "data"
    if args.data:
        from external_dataset import load_bundle
        manifest, _ = load_bundle(data)
    else:
        manifest = build_bundle(repo, data)
    print(json.dumps({name: {key: value for key, value in info.items() if key in ("count", "families")}
                      for name, info in manifest["splits"].items()}), flush=True)
    training, validation = read_rows(data / "train.jsonl"), read_rows(data / "validation.jsonl")
    started = time.perf_counter()
    encoder = FrozenEncoder(default_cache())
    torch = encoder.torch
    torch.cuda.reset_peak_memory_stats()
    load_seconds = time.perf_counter() - started
    extraction_start = time.perf_counter()
    train_features, _ = encoder.encode_rows(training)
    val_features, _ = encoder.encode_rows(validation)
    extraction_seconds = time.perf_counter() - extraction_start
    labels = list(CLASSES)
    targets = lambda rows: torch.tensor([labels.index(row["label"]) for row in rows], dtype=torch.long)
    tensors, fit = fit_head(torch, train_features, targets(training), val_features, targets(validation))
    save_file({name: tensor.contiguous() for name, tensor in tensors.items()}, str(output / "head.safetensors"))
    validation_report, _ = score_rows(torch, tensors, val_features, validation)
    test_rows = read_rows(data / "test.jsonl")
    test_features, _ = encoder.encode_rows(test_rows)
    test_report, test_outputs = score_rows(torch, tensors, test_features, test_rows)
    (output / "test-predictions.jsonl").write_text(
        "".join(json.dumps(row, ensure_ascii=False) + "\n" for row in test_outputs), encoding="utf-8")
    metadata = {
        "schema_version": 1, "model": MODEL_ID, "revision": REVISION,
        "feature_schema": FEATURE_SCHEMA, "max_tokens": encoder.max_tokens,
        "labels": labels, "dimension": encoder.dimension,
        "head_sha256": hashlib.sha256((output / "head.safetensors").read_bytes()).hexdigest(),
        "head_bytes": (output / "head.safetensors").stat().st_size,
        "trainable_parameters": sum(tensors[key].numel() for key in ("weight", "bias")),
        "backbone_trainable_parameters": sum(p.numel() for p in encoder.model.parameters() if p.requires_grad),
        "selection": "Minimum validation cross-entropy, 400 epochs maximum, patience 40; test never used for selection",
        "hyperparameters": {"seed": 42, "learning_rate": 0.003, "weight_decay": 0.1,
                            "class_weighting": "Inverse train class count", "std_floor": 0.05},
        "dataset": manifest, "load_seconds": load_seconds,
        "feature_extraction_train_validation_seconds": extraction_seconds,
        "training": {key: value for key, value in fit.items() if key != "history"},
        "validation": validation_report, "test": test_report,
        "device": torch.cuda.get_device_name(0), "torch": torch.__version__,
        "peak_gpu_allocated_bytes": torch.cuda.max_memory_allocated(),
        "metric_contract": manifest.get("metric_contract", {
            "accuracy": "Argmax agrees with author-provided task kind; all included rows",
            "anchor": "After checkpoint selection on validation; test encoded and scored once",
            "population": "Synthetic held-out scenario families with paired English/Chinese and three wrappers",
            "excludes": "Real customer traffic, execution success, costs, and per-request runtime latency",
            "statistical_unit": "Scenario family; six variants per family are correlated",
        }),
        "limitations": manifest["limitations"] + [
            "Only the linear head changed; the DeepSeek backbone remains frozen.",
            "Head scores are uncalibrated; no automatic routing threshold is established.",
            "Feature extraction used the full frozen backbone; a tiny head does not mean a tiny total model.",
        ],
    }
    (output / "metadata.json").write_text(json.dumps(metadata, indent=2), encoding="utf-8")
    (output / "training-history.json").write_text(json.dumps(fit["history"], indent=2), encoding="utf-8")
    print(json.dumps({"checkpoint": str(output), "test_accuracy": test_report["accuracy"],
                      "validation_accuracy": validation_report["accuracy"],
                      "parameters": metadata["trainable_parameters"],
                      "selected_epoch": fit["selected_epoch"]}, indent=2), flush=True)


if __name__ == "__main__":
    main()
