import argparse
import json
import subprocess
import tempfile
from pathlib import Path


def run(runner, store, commands):
    result = subprocess.run(
        [str(runner), "--runtime", "--store", str(store)],
        input="".join(json.dumps(command) + "\n" for command in commands),
        text=True, capture_output=True, check=True,
    )
    return [json.loads(line) for line in result.stdout.splitlines()]


def candidate(packet):
    return json.dumps({
        "options": {row["id"]: row["status"] for row in packet["options"]},
        "frontier": packet["pareto_frontier"],
        "comparison_state": packet["comparison_state"],
        "recommended_option": None,
    })


def check(snapshot, answer):
    return {"action": "check", "arguments": {
        "snapshot_id": snapshot, "candidate_json": answer,
    }}


def evaluate(runner):
    request = json.loads(Path(__file__).with_name("native-request.json").read_text())
    with tempfile.TemporaryDirectory(prefix="reasonix-decision-") as directory:
        store = Path(directory) / "snapshots.sqlite"
        original = run(runner, store, [{"action": "compare", "arguments": request}])[0]
        old_id = original["snapshot_id"]
        revised = {
            "id": "a-price-v2", "key": "a.monthly_cost",
            "text": "Authored revised fixture: A costs 150 per month.",
            "source": {"uri": "fixture://plan-a", "version": "v2"},
            "observed_at": "2026-09-30T12:00:00Z", "supersedes": ["a-price"],
            "fact": {"metric": "monthly_cost", "value": "150", "unit": "USD/month"},
        }
        restored, updated = run(runner, store, [
            check(old_id, candidate(original)),
            {"action": "update", "arguments": {
                "snapshot_id": old_id, "as_of": "2026-09-30T12:00:00Z",
                "evidence": [revised], "metric_updates": [{
                    "option_id": "a", "metric": "monthly_cost",
                    "evidence_ids": ["a-price-v2"],
                }],
            }},
        ])
        replay = run(runner, store, [
            check(old_id, candidate(original)),
            check(updated["snapshot_id"], candidate(original)),
            check(updated["snapshot_id"], candidate(updated)),
        ])
        assertions = {
            "restored_old_answer_accepted": restored["status"] == "answer_accepted",
            "old_version_unchanged": replay[0]["basis"] == restored["basis"],
            "new_version_linked": updated["parent_snapshot_id"] == old_id,
            "old_answer_rejected_on_new_version": replay[1]["status"] == "computed_fallback",
            "new_answer_accepted": replay[2]["status"] == "answer_accepted",
            "budget_preserved": replay[2]["basis"]["options"][0]["constraint_checks"][0]["verdict"] == "fail",
        }
        if not all(assertions.values()):
            raise AssertionError(assertions)
        return {
            "scope": "One authored two-option fixture across three separate native processes; no model calls. Checks measure persistence and structured consistency, not source truth, generalization, or cost savings.",
            "processes": 3, "assertions": assertions,
            "original": original, "updated": updated, "deliveries": replay,
        }


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--runner", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    report = evaluate(args.runner.resolve())
    args.output.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(json.dumps(report["assertions"]))
