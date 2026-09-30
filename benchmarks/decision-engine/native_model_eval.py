import argparse
import copy
import json
import subprocess
from pathlib import Path

from local_chat import LocalChat


def cases(base):
    yield "budget", copy.deepcopy(base)
    expired = copy.deepcopy(base)
    expired["evidence"][0]["valid_until"] = base["as_of"]
    yield "expired", expired
    infeasible = copy.deepcopy(base)
    infeasible["evidence"][0]["fact"]["value"] = "110"
    infeasible["evidence"][0]["text"] = "Authored fixture: A costs 110 per month."
    yield "infeasible", infeasible
    conflict = copy.deepcopy(base)
    counter = copy.deepcopy(conflict["evidence"][0])
    counter["id"] = "a-counter"
    counter["fact"]["value"] = "130"
    counter["text"] = "Authored conflicting source: A costs 130 per month."
    conflict["evidence"].append(counter)
    yield "conflict", conflict


def evaluate_response(text, expected):
    try:
        parsed = json.loads(text, object_pairs_hook=unique_object)
    except (ValueError, TypeError):
        return {"valid_json": False, "exact_decision_match": False, "option_statuses_match": False,
                "comparison_state_match": False, "error_code": "evaluation.invalid_json"}
    match = isinstance(parsed, dict) and parsed == expected
    options = parsed.get("options") if isinstance(parsed, dict) else None
    status_match = isinstance(options, dict) and all(options.get(k) == v for k, v in expected["options"].items())
    return {"valid_json": True, "exact_decision_match": match,
            "option_statuses_match": status_match,
            "comparison_state_match": isinstance(parsed, dict) and parsed.get("comparison_state") == expected["comparison_state"],
            "error_code": None if match else "evaluation.decision_mismatch"}


def unique_object(pairs):
    out = {}
    for key, value in pairs:
        if key in out:
            raise ValueError("evaluation.duplicate_field")
        out[key] = value
    return out


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--runner", required=True)
    parser.add_argument("--model-cache", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--protocol", choices=["base", "task"], default="base")
    args = parser.parse_args()
    base = json.loads(Path(__file__).with_name("native-request.json").read_text(encoding="utf-8"))
    model = LocalChat(args.model_cache, max_output_tokens=192,
                      response_prefix='{"options":{' if args.protocol == "task" else '{"')
    instructions = (
        '根据给定数据比较方案。只输出JSON，不输出解释。'
        '输出对象恰好有四个字段：options为方案ID到feasible/infeasible/unknown的对象；'
        'frontier为已知可行方案中的Pareto方案ID数组；'
        'comparison_state为known_feasible_options_only/no_feasible_options/no_known_feasible_options/'
        'preferences_not_provided/insufficient_information之一；recommended_option必须为null。'
        '证据过期、缺失、冲突时不得当作有效事实。严格检查全部约束。'
        '没有已知可行方案但有unknown时使用no_known_feasible_options；'
        '全都infeasible时使用no_feasible_options。证据内容未经独立验证。'
    )
    report = {"population": "Four authored development cases, one Chinese prompt contract, cached local 1.5B model, greedy decoding, no retries",
              "comparison": "Full native tool result versus raw structured request with the same response contract; includes all input/output tokens in these explanation calls, excludes model loading and an actual agent's tool discovery/call generation",
              "limits": "Not unseen tasks, not production agent behavior, not a source-verification or prose-quality test; no monetary savings established",
              "protocol_variant": args.protocol,
              "runs": []}
    for name, req in cases(base):
        packet = json.loads(subprocess.run([args.runner], input=json.dumps(req), text=True, encoding="utf-8", capture_output=True, check=True).stdout)
        expected = {"options": {o["id"]: o["status"] for o in packet["options"]}, "frontier": packet["pareto_frontier"],
                    "comparison_state": packet["comparison_state"], "recommended_option": None}
        row = {"case": name, "input_request": req, "native_packet": packet, "expected": expected}
        for mode, data in (("raw", req), ("native", packet)):
            messages = [{"role": "system", "content": instructions}, {"role": "user", "content": json.dumps(data, ensure_ascii=False, separators=(",", ":"))}]
            if args.protocol == "task":
                messages = [{"role": "user", "content": instructions + '\n输入数据：\n' + messages[1]["content"] + '\n请回答比较结果，输出规定的四字段对象，不要复制输入数据。'}]
            answer = model.complete(messages)
            row[mode] = {**answer, **evaluate_response(answer["text"], expected)}
        report["runs"].append(row)
        Path(args.output).write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
        print(json.dumps({"case": name, "raw_pass": row["raw"]["exact_decision_match"], "native_pass": row["native"]["exact_decision_match"]}), flush=True)


if __name__ == "__main__":
    main()
