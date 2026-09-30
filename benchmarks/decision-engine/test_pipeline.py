import json
import tempfile
import unittest
from pathlib import Path

from contract import DecisionError
from demo import scenario
from engine import make_packet
from interpretation import parse_plan
from pipeline import run_flow
from projection import choose_projection, parse_explanation
from state import State


def plan(**updates):
    return json.dumps({"topic_ids": ["launch"], "focus_option_ids": ["c"], "needs_information": [], **updates})


def answer():
    return {"summary": "Compare feasible options with their tradeoffs.",
            "option_notes": [{"option_id": identity, "text": "Consult the cost and feasibility evidence.",
                              "evidence_ids": [identity + "/cost"]} for identity in ("a", "b", "c")],
            "limitations": ["Authored estimates; not independently verified."]}


class ScriptedChat:
    def __init__(self, replies):
        self.replies = list(replies)
        self.messages = []

    def count(self, messages):
        return len(json.dumps(messages))

    def complete(self, messages):
        self.messages.append(messages)
        return {"text": self.replies.pop(0), "usage": {"input_tokens": 10, "output_tokens": 5},
                "finish_reason": "eos", "latency_seconds": 0.01}


class PipelineTests(unittest.TestCase):
    def test_host_constraints_and_options_cannot_be_removed_by_plan(self):
        catalog = scenario()
        _, request = parse_plan(plan(), "Compare alternatives", catalog)
        self.assertEqual(request["constraints"], catalog["constraints"])
        self.assertEqual(request["options"], catalog["options"])
        self.assertEqual(request["preferences"], catalog["preferences"])
        with self.assertRaises(DecisionError) as caught:
            parse_plan(plan(constraints=[]), "Compare", catalog)
        self.assertEqual(caught.exception.code, "interpretation.schema_invalid")

    def test_unknown_semantic_id_rejected(self):
        with self.assertRaises(DecisionError) as caught:
            parse_plan(plan(topic_ids=["invented-topic"]), "Compare", scenario())
        self.assertEqual(caught.exception.code, "interpretation.topic_unknown")

    def test_duplicate_model_fields_are_rejected(self):
        raw = '{"topic_ids":[],"topic_ids":["launch"],"focus_option_ids":[],"needs_information":[]}'
        with self.assertRaises(DecisionError) as caught:
            parse_plan(raw, "Compare", scenario())
        self.assertEqual(caught.exception.code, "model.json_duplicate_key")

    def test_repair_input_budget_stops_before_extra_inference(self):
        chat = ScriptedChat(["bad"])
        chat.count = lambda messages: 1 if len(chat.messages) == 0 else 1000
        with tempfile.TemporaryDirectory() as folder:
            state = State(Path(folder) / "state.sqlite")
            try:
                result = run_flow(chat, scenario(), "Compare", state, max_input_tokens=100)
            finally:
                state.close()
        self.assertEqual(result["status"], "input_budget_exceeded")
        self.assertEqual(result["pipeline_usage"]["calls"], 1)
        self.assertEqual(result["interpretation"]["error_code"], "interpretation.input_budget_exceeded")

    def test_repairs_are_charged_to_full_pipeline_comparison(self):
        chat = ScriptedChat(["not JSON", plan(), json.dumps(answer()), json.dumps(answer())])
        with tempfile.TemporaryDirectory() as folder:
            state = State(Path(folder) / "state.sqlite")
            try:
                result = run_flow(chat, scenario(), "Compare options", state, max_input_tokens=100000, compare=True)
            finally:
                state.close()
        self.assertEqual(result["status"], "ready")
        self.assertEqual(result["pipeline_usage"]["calls"], 3)
        self.assertEqual(result["comparison"]["pipeline_total_tokens"], 45)
        self.assertEqual(result["comparison"]["baseline_total_tokens"], 15)
        self.assertIsNone(result["comparison"]["answer_quality_equivalent"])
        self.assertEqual(result["interpretation"]["traces"][0]["error_code"], "interpretation.json_invalid")
        self.assertIn("computed", result["projection"]["policy"])
        self.assertIn("budget-limit", json.dumps(chat.messages[2]))

    def test_failed_interpretation_is_not_persisted_or_spoken(self):
        chat = ScriptedChat(["bad", "bad again"])
        with tempfile.TemporaryDirectory() as folder:
            state = State(Path(folder) / "state.sqlite")
            try:
                result = run_flow(chat, scenario(), "Compare", state)
                self.assertEqual(result["status"], "interpretation_failed")
                self.assertEqual(state.db.execute("SELECT COUNT(*) FROM records").fetchone()[0], 0)
            finally:
                state.close()
        self.assertNotIn("explanation", result)

    def test_unavailable_question_returns_information_state(self):
        chat = ScriptedChat([plan(needs_information=["request.outside_catalog"])])
        with tempfile.TemporaryDirectory() as folder:
            state = State(Path(folder) / "state.sqlite")
            try:
                result = run_flow(chat, scenario(), "An unsupported task", state)
            finally:
                state.close()
        self.assertEqual(result["status"], "needs_information")
        self.assertNotIn("packet", result)

    def test_fabricated_citation_and_missing_option_are_rejected(self):
        request = scenario()
        packet = make_packet(request)
        value = answer()
        value["option_notes"][0]["evidence_ids"] = ["fabricated"]
        with self.assertRaises(DecisionError) as caught:
            parse_explanation(json.dumps(value), request, packet)
        self.assertEqual(caught.exception.code, "explanation.source_unknown")
        value = answer()
        value["option_notes"].pop()
        with self.assertRaises(DecisionError) as caught:
            parse_explanation(json.dumps(value), request, packet)
        self.assertEqual(caught.exception.code, "explanation.option_missing")

    def test_budget_never_silently_truncates_projection(self):
        request = scenario()
        result = choose_projection(ScriptedChat([]), request, make_packet(request), max_input_tokens=1)
        self.assertEqual(result["status"], "input_budget_exceeded")
        self.assertIn("a/cost", json.dumps(result["messages"]))


if __name__ == "__main__":
    unittest.main()
