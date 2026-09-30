import hashlib
import json
from collections import Counter
from pathlib import Path

from corpus import CLASSES, fingerprint, read_corpus
from scenarios import SCENARIOS


def write_rows(path, rows):
    path.write_text("".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows), encoding="utf-8")


def audit_splits(splits):
    owners, families = {}, {}
    for split, rows in splits.items():
        for row in rows:
            digest = fingerprint(row["prompt"])
            if digest in owners:
                raise ValueError("Repeated normalized prompt in dataset bundle")
            owners[digest] = split
            family = row["family"]
            if family in families and families[family] != split:
                raise ValueError("Scenario family crosses splits")
            families[family] = split
    return {"exact_prompt_overlap": 0, "family_overlap": 0}


def build_bundle(repo, output):
    output = Path(output)
    output.mkdir(parents=True, exist_ok=True)
    splits = {name: [] for name in ("train", "validation", "test")}
    wrappers = {
        "en": ["{}", "Request:\n{}", "Please handle the following task:\n{}"],
        "zh": ["{}", "任务：\n{}", "请处理下面的需求：\n{}"],
    }
    if set(SCENARIOS) != set(CLASSES):
        raise ValueError("Scenario labels differ from task-kind contract")
    for label, scenarios in SCENARIOS.items():
        if len(scenarios) != 12:
            raise ValueError("Each label requires 12 scenario families")
        for index, pair in enumerate(scenarios):
            split = "train" if index < 8 else "validation" if index < 10 else "test"
            family = f"authored/{label}/{index:02d}"
            for language, text in zip(("en", "zh"), pair):
                for variant, wrapper in enumerate(wrappers[language]):
                    prompt = wrapper.format(text)
                    splits[split].append({
                        "id": f"{family}/{language}/{variant}", "family": family,
                        "prompt": prompt, "label": label, "language": language,
                        "source": "Codex-authored synthetic scenario; not independent annotation",
                        "sha256": fingerprint(prompt),
                    })
    existing, excluded = read_corpus(repo / "benchmarks/train/tasks")
    for row in existing:
        splits["train"].append({**row, "family": "existing-train/" + row["id"],
                                "language": "en", "source": "Existing training-only task.toml"})
    audit = audit_splits(splits)
    regression, _ = read_corpus(repo / "benchmarks/e2e/tasks")
    if {row["sha256"] for rows in splits.values() for row in rows} & {
            row["sha256"] for row in regression}:
        raise ValueError("Authored bundle duplicates regression prompts")
    for name, rows in splits.items():
        write_rows(output / f"{name}.jsonl", rows)
    manifest = {
        "schema_version": 1, "labels": list(CLASSES), "audit": audit,
        "split_rule": "Per label: families 0-7 train, 8-9 validation, 10-11 test",
        "splits": {name: {"count": len(rows), "families": len({r["family"] for r in rows}),
                          "classes": dict(Counter(r["label"] for r in rows)),
                          "languages": dict(Counter(r["language"] for r in rows)),
                          "sha256": hashlib.sha256((output / f"{name}.jsonl").read_bytes()).hexdigest()}
                   for name, rows in splits.items()},
        "existing_training_excluded": excluded,
        "limitations": [
            "Six variants per authored family share meaning; rows are not independent observations.",
            "Language and wrapper variants stay together; wrapper styles are shared across splits.",
            "Scenario subjects differ across splits, but no claim of complete semantic separation is made.",
            "Labels are author-provided task kinds, not measured cheapest successful routes.",
            "The previous 51 e2e tasks are a development regression set, not a fresh test set.",
            "Synthetic held-out performance does not establish performance on customer requests.",
        ],
    }
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2), encoding="utf-8")
    return manifest


if __name__ == "__main__":
    repo = Path(__file__).resolve().parents[2]
    print(json.dumps(build_bundle(repo, repo / "temp/decision-model/head-data"), indent=2))
