import argparse
import hashlib
import json
from collections import Counter
from pathlib import Path

from corpus import CLASSES, fingerprint
from model_config import MODEL_ID, REVISION
from training_data import audit_splits, build_bundle, write_rows


def build_github_bundle(repo, snapshot_dir, output):
    from transformers import AutoTokenizer
    from encoder import default_cache

    output.mkdir(parents=True, exist_ok=True)
    synthetic_dir = output / "synthetic-reference"
    synthetic_manifest = build_bundle(repo, synthetic_dir)
    annotations = json.loads((Path(__file__).parent / "github_annotations.json").read_text(encoding="utf-8"))
    tokenizer = AutoTokenizer.from_pretrained(
        MODEL_ID, revision=REVISION,
        cache_dir=str(default_cache()), local_files_only=True, trust_remote_code=False)
    real = {name: [] for name in ("train", "validation", "test")}
    excluded, sources = [], []
    for repository, review in annotations["repositories"].items():
        path = snapshot_dir / (repository.replace("/", "--") + ".json")
        snapshot = json.loads(path.read_text(encoding="utf-8"))
        candidates = {}
        for pr in snapshot["pull_requests"]:
            for issue in pr["closingIssuesReferences"]["nodes"]:
                candidates.setdefault(str(issue["number"]), (issue, pr))
        for number, (label, rationale) in review["issues"].items():
            issue, pr = candidates[number]
            if issue["url"] != f"https://github.com/{repository}/issues/{number}":
                raise ValueError("Cross-repository issue reference requires separate review")
            prompt = issue["title"] + "\n\n" + issue["body"]
            tokens = len(tokenizer.encode(prompt + tokenizer.eos_token, add_special_tokens=False))
            if tokens > 1024:
                excluded.append({"url": issue["url"], "reason": "input_token_limit", "tokens": tokens})
                continue
            row = {
                "id": f"github/{repository}/{number}", "family": f"github/{repository}/{number}",
                "repository": repository, "prompt": prompt, "label": label, "language": "en",
                "sha256": fingerprint(prompt), "source": "GitHub issue, Codex-reviewed task-kind label",
                "source_url": issue["url"], "review_rationale": rationale,
                "provenance": {"retrieved_at": snapshot["retrieved_at"], "issue_updated_at": issue["updatedAt"],
                               "linked_pr_url": pr["url"], "base_ref_oid_at_snapshot": pr["baseRefOid"],
                               "merge_commit": (pr["mergeCommit"] or {}).get("oid"),
                               "changed_paths": [f["path"] for f in pr["files"]["nodes"]],
                               "repository_license_spdx": snapshot["repository_license_spdx"],
                               "execution_verified": False, "issue_text_rights_reviewed": False},
            }
            real[review["split"]].append(row)
        sources.append({"repository": repository, "split": review["split"],
                        "snapshot_sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
                        "repository_license_spdx": snapshot["repository_license_spdx"]})
    audit_splits(real)
    splits = {
        "train": [json.loads(line) for line in (synthetic_dir / "train.jsonl").read_text(encoding="utf-8").splitlines()] + real["train"],
        "validation": real["validation"], "test": real["test"],
    }
    audit = audit_splits(splits)
    for name, rows in splits.items():
        if not rows:
            raise ValueError("Empty dataset split")
        write_rows(output / f"{name}.jsonl", rows)
    manifest = {
        "schema_version": 1, "labels": list(CLASSES), "audit": audit,
        "split_rule": "GitHub repositories assigned wholly to one split before training; synthetic training rows retained",
        "sources": sources, "excluded": excluded,
        "annotation": {"annotator": annotations["annotator"], "policy": annotations["policy"]},
        "synthetic_training_manifest": synthetic_manifest,
        "splits": {name: {"count": len(rows), "families": len({r["family"] for r in rows}),
                          "classes": dict(Counter(r["label"] for r in rows)),
                          "languages": dict(Counter(r["language"] for r in rows)),
                          "sha256": hashlib.sha256((output / f"{name}.jsonl").read_bytes()).hexdigest(),
                          "github_count": len(real[name])} for name, rows in splits.items()},
        "metric_contract": {
            "population": "Manually selected linked GitHub issue reports within seven Python projects",
            "accuracy": "Agreement with retrospective single-Codex task-kind labels",
            "anchor": "One issue title and current body at retrieval, excluding PR metadata from model input",
            "excludes": "Task execution, reproduction, costs, customer requests and unsupported category coverage",
        },
        "limitations": [
            "Small convenience sample, mostly bug reports; not a balanced eight-category benchmark.",
            "Labels use retrospective PR scope and a single Codex reviewer, not independent human adjudication.",
            "Current issue bodies may contain suggested fixes or edits after resolution; pre-resolution text was not reconstructed.",
            "Backbone pretraining contamination is unknown, especially for older public issues.",
            "Merged PR references are evidence of linkage, not locally verified fixes or tests.",
            "baseRefOid is the PR base ref at retrieval, not necessarily the historical pre-fix checkout; merge parent needs verification before replay.",
            "Repository code license does not alone establish rights to every contributor's issue prose; commercial data-rights review remains outstanding.",
            "No personal author profiles or private repositories were collected. Issue bodies remain untrusted data.",
            "No claim of full semantic independence across projects, nor of real-task cost savings.",
        ],
    }
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2), encoding="utf-8")
    return manifest


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--snapshot", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if (args.output / "manifest.json").exists():
        parser.error("Dataset already exists; choose a new output")
    manifest = build_github_bundle(Path(__file__).resolve().parents[2], args.snapshot, args.output)
    print(json.dumps({"splits": manifest["splits"], "excluded": manifest["excluded"]}, indent=2))


if __name__ == "__main__":
    main()
