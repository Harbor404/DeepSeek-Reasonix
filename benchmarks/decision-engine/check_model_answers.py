import argparse
import json
import subprocess
from pathlib import Path


def replay(runner, recorded):
    out = {"population": "Eight recorded outputs on four authored development fixtures; replay only, no new model calls",
           "scope": "Structured consistency against the original input recorded by the local evaluation host; no source/prose verification or final-chat filtering",
           "accepted": 0, "fallback": 0, "canonical_deliveries_match": 0, "runs": []}
    for row in recorded["runs"]:
        for mode in ("raw", "native"):
            request = {"request": row["input_request"], "candidate_json": row[mode]["text"]}
            completed = subprocess.run([runner, "--check"], input=json.dumps(request), text=True, encoding="utf-8", capture_output=True, check=True)
            delivery = json.loads(completed.stdout)
            if delivery["status"] not in ("answer_accepted", "computed_fallback"):
                raise ValueError("replay.invalid_delivery")
            canonical_match = delivery["decision"] == row["expected"]
            out["accepted" if delivery["status"] == "answer_accepted" else "fallback"] += 1
            out["canonical_deliveries_match"] += canonical_match
            out["runs"].append({"case": row["case"], "mode": mode,
                                "host_generation_finish_reason": row[mode]["finish_reason"],
                                "canonical_match": canonical_match, "delivery": delivery})
    return out


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--runner", required=True)
    parser.add_argument("--recorded", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    recorded = json.loads(Path(args.recorded).read_text(encoding="utf-8"))
    report = replay(args.runner, recorded)
    Path(args.output).write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({key: report[key] for key in ("accepted", "fallback", "canonical_deliveries_match")}))


if __name__ == "__main__":
    main()
