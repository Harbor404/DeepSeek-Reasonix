import hashlib
import json
import tomllib
from collections import Counter
from pathlib import Path


CLASSES = {
    "atomic-bugfix": "Repair one localized implementation defect.",
    "multi-file-bugfix": "Repair a defect involving coordinated changes across files.",
    "failing-test-diagnosis": "Diagnose a reported failing test or execution failure and fix its cause.",
    "repo-exploration": "Explore a repository and answer a question about its implementation.",
    "read-only-output": "Produce an analysis or handover without changing implementation.",
    "refactor": "Restructure implementation while preserving its behavior.",
    "codegen": "Implement a requested function or extend an existing public interface.",
    "api-integration": "Integrate a component with an explicit API and lifecycle contract.",
}


def fingerprint(prompt):
    return hashlib.sha256(" ".join(prompt.split()).encode()).hexdigest()


def read_corpus(root):
    rows, excluded = [], []
    for path in sorted(Path(root).glob("*/task.toml")):
        task = tomllib.loads(path.read_text(encoding="utf-8"))
        if task["class"] not in CLASSES:
            excluded.append({"id": path.parent.name, "class": task["class"]})
            continue
        rows.append({"id": path.parent.name, "prompt": task["prompt"],
                     "label": task["class"], "sha256": fingerprint(task["prompt"])})
    return rows, excluded


def export_corpora(repo, output):
    output = Path(output)
    output.mkdir(parents=True, exist_ok=True)
    train, train_excluded = read_corpus(repo / "benchmarks/train/tasks")
    evaluation, eval_excluded = read_corpus(repo / "benchmarks/e2e/tasks")
    overlap = {row["sha256"] for row in train} & {row["sha256"] for row in evaluation}
    if overlap:
        raise ValueError("Training/evaluation prompts overlap")
    for name, rows in (("train", train), ("evaluation", evaluation)):
        (output / f"{name}.jsonl").write_text(
            "".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows),
            encoding="utf-8")
    manifest = {
        "schema_version": 1,
        "labels": CLASSES,
        "label_source": "Existing task.toml class; task-kind proxy, not optimal model routing",
        "train": {"count": len(train), "excluded": train_excluded},
        "evaluation": {"count": len(evaluation), "excluded": eval_excluded,
                       "counts": dict(Counter(row["label"] for row in evaluation))},
        "exact_normalized_prompt_overlap": 0,
        "semantic_overlap": "Not audited; benchmark families may share patterns",
    }
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2), encoding="utf-8")
    return manifest


def metrics(rows):
    if not rows:
        raise ValueError("No evaluation rows")
    counts = Counter(row["label"] for row in rows)
    per_class = {}
    for label, count in sorted(counts.items()):
        selected = [row for row in rows if row["label"] == label]
        correct = sum(row.get("prediction") == label for row in selected)
        per_class[label] = {"count": count, "correct": correct, "recall": correct / count}
    attempted = [row for row in rows if "latency_ms" in row]
    valid = sum(row.get("prediction") is not None for row in rows)
    correct = sum(row.get("prediction") == row["label"] for row in rows)
    latencies = sorted(row["latency_ms"] for row in attempted)

    def percentile(fraction):
        if not latencies:
            return None
        return latencies[min(len(latencies) - 1, int((len(latencies) - 1) * fraction + 0.5))]

    return {
        "count": len(rows), "attempted": len(attempted), "correct": correct,
        "accuracy": correct / len(rows),
        "balanced_accuracy": sum(value["recall"] for value in per_class.values()) / len(per_class),
        "coverage": valid / len(rows),
        "majority_baseline_accuracy": max(counts.values()) / len(rows),
        "latency_ms_p50": percentile(0.5), "latency_ms_p95": percentile(0.95),
        "per_class": per_class,
    }
