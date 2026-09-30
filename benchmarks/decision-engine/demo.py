import argparse
import copy
import json
from pathlib import Path


def evidence(identity, key, topic, metric=None, value=None, unit=None, **extra):
    row = {"id": identity, "key": key, "topic_id": topic,
           "text": f"Recorded {key}: {value} {unit}." if metric else "Background observation.",
           "source": {"uri": "fixture://planning/" + identity, "version": "1"},
           "observed_at": "2026-09-28T00:00:00Z", **extra}
    if metric:
        row["fact"] = {"metric": metric, "value": value, "unit": unit}
    return row


def scenario():
    rows = [evidence("budget", "limits/budget", "launch", "cost", 1000, "USD"),
            evidence("deadline", "limits/deadline", "launch", "days", 14, "day")]
    options = []
    for identity, name, cost, days, impact in (("a", "Improve onboarding", 700, 7, 20),
                                              ("b", "Launch enterprise dashboard", 1500, 21, 40),
                                              ("c", "Improve search", 900, 10, 30)):
        metrics = {}
        for metric, value, unit in (("cost", cost, "USD"), ("days", days, "day"), ("impact", impact, "score")):
            source_id = identity + "/" + metric
            rows.append(evidence(source_id, source_id, "launch", metric, value, unit))
            metrics[metric] = [source_id]
        options.append({"id": identity, "name": name, "metrics": metrics})
    for index in range(24):
        row = evidence(f"archive/{index}", f"archive/{index}", "unrelated-archive")
        row["text"] = (f"Unrelated authored archive entry {index}: prior office relocation, equipment inventory, "
                       "historical newsletter drafts and completed meeting logistics. " * 8)
        rows.append(row)
    return {
        "schema_version": 1, "as_of": "2026-09-29T00:00:00Z",
        "question": "Which product improvement can fit the launch budget and deadline, and what are the tradeoffs?",
        "required_topics": ["launch"], "evidence": rows,
        "memory": [{"id": "decision/launch-limits", "topic_id": "launch",
                    "text": "The launch budget is 1000 USD and the deadline is 14 days.",
                    "source": {"uri": "fixture://planning/decision", "version": "1"},
                    "observed_at": "2026-09-28T00:00:00Z", "evidence_ids": ["budget", "deadline"]}],
        "options": options,
        "constraints": [{"id": "budget-limit", "metric": "cost", "op": "lte", "limit": 1000,
                         "unit": "USD", "evidence_ids": ["budget"]},
                        {"id": "deadline-limit", "metric": "days", "op": "lte", "limit": 14,
                         "unit": "day", "evidence_ids": ["deadline"]}],
        "preferences": [{"metric": "cost", "unit": "USD", "direction": "min"},
                        {"metric": "impact", "unit": "score", "direction": "max"}],
    }


def cases():
    base = scenario()
    result = {"normal": base}
    compact = copy.deepcopy(base)
    compact["evidence"] = [r for r in compact["evidence"] if r["topic_id"] == "launch"]
    result["compact-input"] = compact
    expired = copy.deepcopy(base)
    expired["evidence"][2]["valid_until"] = "2026-09-28T12:00:00Z"
    result["expired"] = expired
    conflict = copy.deepcopy(base)
    conflict["evidence"].append(evidence("a/cost-other", "a/cost", "launch", "cost", 1200, "USD"))
    result["conflict"] = conflict
    missing = copy.deepcopy(base)
    missing["options"][0]["metrics"]["cost"] = ["missing-cost"]
    result["missing"] = missing
    mismatch = copy.deepcopy(base)
    mismatch["evidence"][2]["fact"]["unit"] = "EUR"
    result["unit-mismatch"] = mismatch
    revised = copy.deepcopy(base)
    revised["evidence"].append(evidence("a/cost-v2", "a/cost", "launch", "cost", 1200, "USD",
                                        supersedes=["a/cost"], observed_at="2026-09-29T00:00:00Z"))
    revised["options"][0]["metrics"]["cost"] = ["a/cost-v2"]
    result["revised"] = revised
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    for name, request in cases().items():
        (args.output / f"{name}.json").write_text(json.dumps(request, indent=2), encoding="utf-8")
    print("Generated seven authored fixtures; not customer data.")


if __name__ == "__main__":
    main()
