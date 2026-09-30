import argparse
import json
import time
from pathlib import Path

from contract import DecisionError, digest, validate
from engine import make_packet
from interpretation import interpret
from local_chat import LocalChat
from projection import choose_projection, explain, explanation_messages
from state import State


def usage(traces):
    return {"input_tokens": sum(r["usage"]["input_tokens"] for r in traces),
            "output_tokens": sum(r["usage"]["output_tokens"] for r in traces),
            "calls": len(traces), "model_seconds": sum(r["latency_seconds"] for r in traces)}


def run_flow(chat, catalog, question, state, attempts=2, max_input_tokens=10000, compare=False):
    started = time.perf_counter()
    merged = state.merge(dict(catalog, question=question))
    validate(merged)
    interpretation = interpret(chat, question, merged, attempts, max_input_tokens)
    result = {"interpretation": interpretation, "status": interpretation["status"],
              "catalog_snapshot_sha256": digest(merged), "question": question,
              "attempts_per_stage": attempts, "comparison_order": "Pipeline then direct baseline, no warmup"}
    traces = list(interpretation["traces"])
    if interpretation["status"] == "ready":
        request = interpretation["request"]
        state.ingest(catalog["evidence"], catalog["memory"])
        key, packet = state.cached(request)
        cache_hit = packet is not None
        if packet is None:
            packet = make_packet(request)
            state.save(key, packet)
        projection = choose_projection(chat, request, packet, max_input_tokens)
        result.update({"packet": packet, "packet_cache_hit": cache_hit,
                       "decision_status": "computed_from_supplied_evidence",
                       "projection": {k: v for k, v in projection.items() if k != "messages"},
                       "status": projection["status"]})
        if projection["status"] == "ready":
            explanation = explain(chat, projection["messages"], request, packet, attempts, max_input_tokens)
            result["explanation"] = explanation
            result["status"] = explanation["status"]
            traces.extend(explanation["traces"])
    result["pipeline_usage"] = usage(traces)
    result["pipeline_wall_seconds_excluding_model_load"] = time.perf_counter() - started
    if compare and "packet" in result:
        baseline_started = time.perf_counter()
        messages = explanation_messages(merged, [o["id"] for o in merged["options"]])
        baseline_count = chat.count(messages)
        if baseline_count > max_input_tokens:
            result["baseline"] = {"status": "input_budget_exceeded", "input_tokens": baseline_count}
        else:
            baseline = explain(chat, messages, merged, result["packet"], attempts, max_input_tokens)
            result["baseline"] = {**baseline, "usage": usage(baseline["traces"]),
                                  "wall_seconds_excluding_model_load": time.perf_counter() - baseline_started}
            optimized = result["pipeline_usage"]
            reference = result["baseline"]["usage"]
            opt_tokens = optimized["input_tokens"] + optimized["output_tokens"]
            raw_tokens = reference["input_tokens"] + reference["output_tokens"]
            result["comparison"] = {
                "pipeline_total_tokens": opt_tokens, "baseline_total_tokens": raw_tokens,
                "token_reduction_fraction": 1 - opt_tokens / raw_tokens if raw_tokens else None,
                "both_schema_valid": result["status"] == "ready" and baseline["status"] == "ready",
                "answer_quality_equivalent": None, "cost_reduction": None,
                "excludes": "Model loading, billing rates, upstream fact collection and independent answer-quality review",
                "anchor": "Pipeline includes interpretation, explanation and all repairs; baseline includes direct explanation and all repairs",
            }
    result["limitations"] = [
        "Authored catalog facts and constraints are supplied, not extracted from arbitrary documents.",
        "Interpretation selects existing topics and focus options only; all host-declared constraints, options and preferences remain intact.",
        "Model-proposed needs-information states are not independently verified semantic understanding.",
        "Explanation validation checks structure and references, not correctness of every natural-language claim.",
        "Local distilled model reliability is unestablished; reasoning is explicitly closed for this experiment.",
        "Tokenizer totals do not establish money saved or equivalent decision quality.",
        "No production Reasonix controller, permission or provider integration is changed.",
    ]
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("catalog", type=Path)
    parser.add_argument("--question", required=True)
    parser.add_argument("--state", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--model-cache", type=Path, required=True)
    parser.add_argument("--compare", action="store_true")
    parser.add_argument("--attempts", type=int, default=2)
    parser.add_argument("--max-output-tokens", type=int, default=512)
    parser.add_argument("--max-input-tokens", type=int, default=10000)
    args = parser.parse_args()
    if not 1 <= args.attempts <= 3 or not 1 <= args.max_output_tokens <= 2048 or args.max_input_tokens < 1:
        parser.error("Invalid bounded inference configuration")
    state = None
    try:
        catalog = json.loads(args.catalog.read_text(encoding="utf-8"))
        validate(dict(catalog, question=args.question))
        started = time.perf_counter()
        chat = LocalChat(args.model_cache, args.max_output_tokens)
        load_seconds = time.perf_counter() - started
        state = State(args.state)
        result = run_flow(chat, catalog, args.question, state, args.attempts, args.max_input_tokens, args.compare)
        result.update({"model": chat.model_id, "revision": chat.revision,
                       "model_load_seconds": load_seconds,
                       "generation": "greedy, reasoning prefix explicitly closed, JSON opening prefilled",
                       "max_output_tokens_per_attempt": args.max_output_tokens})
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
        print(json.dumps({k: result[k] for k in ("status", "pipeline_usage", "pipeline_wall_seconds_excluding_model_load")}, indent=2))
        if "comparison" in result:
            print(json.dumps(result["comparison"], indent=2))
        return 0 if result["status"] == "ready" else 2
    except DecisionError as error:
        print(json.dumps({"error": {"code": error.code, "id": error.identity}}))
        return 1
    finally:
        if state:
            state.close()


if __name__ == "__main__":
    raise SystemExit(main())
