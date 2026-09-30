import argparse
import json
import sys
import tempfile
import time
from pathlib import Path

from contract import VERSION, validate
from demo import cases
from engine import make_packet
from run import measure
from state import State


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--tokenizer-cache", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    from transformers import AutoTokenizer
    sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "decision-model"))
    from model_config import MODEL_ID, REVISION
    tokenizer = AutoTokenizer.from_pretrained(MODEL_ID, revision=REVISION,
                                              cache_dir=str(args.tokenizer_cache),
                                              local_files_only=True, trust_remote_code=False)
    expected = {"normal": "feasible", "compact-input": "feasible", "revised": "infeasible",
                "expired": "unknown", "conflict": "unknown", "missing": "unknown", "unit-mismatch": "unknown"}
    reports = []
    with tempfile.TemporaryDirectory() as directory:
        for name, request in cases().items():
            state = State(Path(directory) / (name + ".sqlite"))
            try:
                merged = state.merge(request)
                validate(merged)
                state.ingest(request["evidence"], request["memory"])
                started = time.perf_counter()
                key, prior = state.cached(merged)
                if prior is not None:
                    raise RuntimeError("Unexpected preexisting benchmark cache")
                packet = make_packet(merged)
                state.save(key, packet)
                first_seconds = time.perf_counter() - started
                started = time.perf_counter()
                _, cached = state.cached(merged)
                cached_seconds = time.perf_counter() - started
                if cached != packet:
                    raise RuntimeError("Cache roundtrip differs")
                option_a = next(option for option in packet["options"] if option["id"] == "a")
                if option_a["status"] != expected[name]:
                    raise RuntimeError("Fixture constraint verdict differs")
                references = {i for option in request["options"] for ids in option["metrics"].values() for i in ids}
                references |= {i for constraint in request["constraints"] for i in constraint["evidence_ids"]}
                retained = {r["id"] for r in packet["evidence"]}
                missing = references - retained
                accounted = {issue["id"] for issue in packet["issues"] if issue["code"] == "evidence.missing"}
                if not missing <= accounted:
                    raise RuntimeError("A referenced source disappeared without missing-evidence identity")
                reports.append({"case": name, "fixture_verdict_matches": True,
                                "referenced_evidence_count": len(references),
                                "retained_referenced_count": len(references & retained),
                                "explicitly_missing_count": len(missing), "unaccounted_count": 0,
                                "option_a_status": option_a["status"],
                                "pareto_frontier": packet["comparison"]["pareto_frontier"],
                                "packet_compute_seconds": first_seconds, "cached_lookup_seconds": cached_seconds,
                                "measurements": measure(merged, packet, args.tokenizer_cache, tokenizer)})
            finally:
                state.close()
    report = {"engine_version": VERSION, "cases": reports,
              "metric_contract": {
                  "anchor": "After JSON fixture load, schema validation, state ingestion and tokenizer loading",
                  "population": "Seven authored variants of one product-planning scenario",
                  "token_count": "Canonical raw context versus full decision packet with the same evidence-use instruction, no chat template",
                  "compute_time": "Cache lookup, packet generation and packet persistence; one run per fixture",
                  "reference_retention": "Option metric and constraint references retained or explicitly flagged missing",
                  "excludes": "Natural-language extraction, semantic relevance, real users, provider calls, output tokens, answer quality and total cost",
              },
              "limitations": [
                  "Twenty-four deliberately unrelated, repetitive archive entries inflate the long-context baseline.",
                  "Topic identities, facts, sources and preferences are supplied by the fixture author, not discovered by a model.",
                  "Compact inputs may expand; token reduction is not universally positive.",
                  "Constraint truth is conditional on supplied facts, not independent verification of their sources.",
                  "Expected fixture verdicts verify mechanisms, not decision quality or benchmark generalization.",
                  "No claim of user savings, faster provider responses or commercial readiness.",
              ]}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2), encoding="utf-8")
    print(json.dumps([{ "case": r["case"], "raw_tokens": r["measurements"]["raw_tokens"],
                       "packet_tokens": r["measurements"]["packet_tokens"],
                       "reduction": r["measurements"]["token_reduction_fraction"],
                       "status": r["option_a_status"]} for r in reports], indent=2))


if __name__ == "__main__":
    main()
