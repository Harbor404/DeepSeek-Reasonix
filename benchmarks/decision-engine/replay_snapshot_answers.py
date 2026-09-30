import argparse
import json
import subprocess
import sys
from pathlib import Path


def encoded(value):
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"))


def replay(runner, recorded, tokenizer):
    process = subprocess.Popen([runner, "--runtime"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, text=True, encoding="utf-8", bufsize=1)
    report = {"population": "Eight previously recorded model answers, four authored cases, one native snapshot runtime, no new model calls",
              "metric": "Tokenizer-counted serialized check arguments only; comparison submissions, schemas, chat history, result bodies, model outputs and retries excluded",
              "limits": "This measures avoided retransmission of original requests, not total conversation or monetary savings; in-memory snapshots do not survive restart",
              "accepted": 0, "fallback": 0, "canonical_matches": 0, "legacy_argument_tokens": 0, "snapshot_argument_tokens": 0, "runs": []}

    def invoke(action, arguments):
        process.stdin.write(encoded({"action": action, "arguments": arguments}) + "\n")
        process.stdin.flush()
        line = process.stdout.readline()
        if not line:
            raise RuntimeError("replay.runner_closed")
        return json.loads(line)

    try:
        for row in recorded["runs"]:
            packet = invoke("compare", row["input_request"])
            snapshot_id = packet["snapshot_id"]
            for mode in ("raw", "native"):
                candidate = row[mode]["text"]
                legacy_args = {"request": row["input_request"], "candidate_json": candidate}
                args = {"snapshot_id": snapshot_id, "candidate_json": candidate}
                delivery = invoke("check", args)
                if delivery["status"] not in ("answer_accepted", "computed_fallback"):
                    raise RuntimeError("replay.invalid_delivery")
                matches = delivery["decision"] == row["expected"]
                bound = delivery["basis"]["snapshot_sha256"] == packet["snapshot_sha256"]
                if not bound:
                    raise RuntimeError("replay.snapshot_binding_failed")
                before = len(tokenizer.encode(encoded(legacy_args), add_special_tokens=False))
                after = len(tokenizer.encode(encoded(args), add_special_tokens=False))
                report["legacy_argument_tokens"] += before
                report["snapshot_argument_tokens"] += after
                report["accepted" if delivery["status"] == "answer_accepted" else "fallback"] += 1
                report["canonical_matches"] += matches
                report["runs"].append({"case": row["case"], "mode": mode, "bound": bound,
                                       "canonical_match": matches, "legacy_tokens": before, "snapshot_tokens": after,
                                       "host_generation_finish_reason": row[mode]["finish_reason"], "delivery": delivery})
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
        raise RuntimeError("replay.runner_failed")
    return report


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--runner", required=True)
    parser.add_argument("--recorded", required=True)
    parser.add_argument("--model-cache", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    from transformers import AutoTokenizer
    sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "decision-model"))
    from model_config import MODEL_ID, REVISION
    tokenizer = AutoTokenizer.from_pretrained(MODEL_ID, revision=REVISION, cache_dir=args.model_cache,
                                             local_files_only=True, trust_remote_code=False)
    report = replay(args.runner, json.loads(Path(args.recorded).read_text(encoding="utf-8")), tokenizer)
    report["tokenizer"] = {"model": MODEL_ID, "revision": REVISION}
    Path(args.output).write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({key: report[key] for key in ("accepted", "fallback", "canonical_matches", "legacy_argument_tokens", "snapshot_argument_tokens")}))


if __name__ == "__main__":
    main()
