import argparse
import json
import subprocess
from pathlib import Path


def evaluate(runner, recorded, example):
    process = subprocess.Popen(
        [str(runner), "--runtime"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
        stderr=subprocess.PIPE, text=True, encoding="utf-8", bufsize=1,
    )

    def invoke(action, arguments):
        process.stdin.write(json.dumps({"action": action, "arguments": arguments}) + "\n")
        process.stdin.flush()
        line = process.stdout.readline()
        if not line:
            raise RuntimeError("delivery.runner_closed")
        return json.loads(line)

    rows = []
    try:
        for row in recorded["runs"]:
            packet = invoke("compare", row["input_request"])
            texts = []
            for mode in ("raw", "native"):
                output = invoke("deliver", {
                    "snapshot_id": packet["snapshot_id"],
                    "candidate_json": row[mode]["text"],
                })
                if output["status"] not in ("answer_accepted", "computed_fallback"):
                    raise RuntimeError("delivery.invalid_result")
                if output["decision"] != row["expected"] or output["snapshot_sha256"] != packet["snapshot_sha256"]:
                    raise RuntimeError("delivery.binding_or_decision_mismatch")
                texts.append(output["report"]["text"])
                rows.append({"case": row["case"], "mode": mode, "output": output})
            if texts[0] != texts[1]:
                raise RuntimeError("delivery.candidate_changed_report")
        example.write_text(rows[0]["output"]["report"]["text"], encoding="utf-8")
    finally:
        process.stdin.close()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
        process.stdout.close()
        process.stderr.close()
    if process.returncode != 0:
        raise RuntimeError("delivery.runner_failed")
    return {
        "scope": "Eight previously recorded outputs from four authored cases; no new model calls. Measures canonical decision binding and candidate-independent report generation, not source truth, real-user quality, latency improvement or end-to-end token savings.",
        "cases": 4, "outputs": len(rows), "canonical_matches": len(rows),
        "accepted": sum(row["output"]["status"] == "answer_accepted" for row in rows),
        "fallback": sum(row["output"]["status"] == "computed_fallback" for row in rows),
        "candidate_independent_reports": 4, "additional_report_model_calls": 0,
        "runs": rows,
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--runner", type=Path, required=True)
    parser.add_argument("--recorded", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--example", type=Path, required=True)
    args = parser.parse_args()
    report = evaluate(args.runner.resolve(), json.loads(args.recorded.read_text(encoding="utf-8")), args.example)
    args.output.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(json.dumps({key: value for key, value in report.items() if key != "runs"}))
