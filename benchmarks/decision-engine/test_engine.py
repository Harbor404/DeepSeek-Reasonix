import copy
import tempfile
import unittest
from pathlib import Path

from contract import DecisionError, validate
from demo import cases, evidence, scenario
from engine import make_packet
from run import prompts
from state import State


class DecisionTests(unittest.TestCase):
    def option(self, packet, identity="a"):
        return next(row for row in packet["options"] if row["id"] == identity)

    def test_packet_preserves_required_evidence_and_tradeoffs(self):
        request = scenario()
        packet = make_packet(request)
        retained = {r["id"] for r in packet["evidence"]}
        references = {i for option in request["options"] for ids in option["metrics"].values() for i in ids}
        references |= {i for c in request["constraints"] for i in c["evidence_ids"]}
        self.assertTrue(references <= retained)
        self.assertEqual(packet["comparison"]["pareto_frontier"], ["a", "c"])
        self.assertEqual(self.option(packet, "b")["status"], "infeasible")
        self.assertIsNone(packet["comparison"]["recommended_option"])
        raw, reduced = prompts(request, packet)
        self.assertIn("untrusted", reduced)
        self.assertNotIn("Unrelated authored archive", reduced)
        self.assertIn("Recorded a/cost: 700 USD", reduced)
        self.assertLess(len(reduced), len(raw))

    def test_unknowns_are_never_treated_as_passed(self):
        for name in ("expired", "conflict", "missing", "unit-mismatch"):
            with self.subTest(name=name):
                packet = make_packet(cases()[name])
                self.assertEqual(self.option(packet)["status"], "unknown")
                check = self.option(packet)["constraint_checks"][0]
                self.assertEqual(check["verdict"], "unknown")
        conflict = make_packet(cases()["conflict"])
        self.assertEqual(conflict["conflicts"][0]["evidence_ids"], ["a/cost", "a/cost-other"])

    def test_units_and_limit_provenance_are_checked(self):
        request = scenario()
        request["constraints"][0]["limit"] = 2000
        packet = make_packet(request)
        self.assertEqual(self.option(packet)["status"], "unknown")
        self.assertEqual(packet["constraints"][0]["support"]["code"], "evidence.limit_mismatch")

    def test_equivalent_numeric_spellings_do_not_conflict(self):
        request = scenario()
        request["evidence"].append(evidence("a/cost-confirmed", "a/cost", "launch", "cost", "700.00", "USD"))
        packet = make_packet(request)
        self.assertEqual(self.option(packet)["status"], "feasible")
        self.assertEqual(packet["conflicts"], [])

    def test_future_revision_does_not_supersede_current_fact(self):
        request = cases()["revised"]
        request["as_of"] = "2026-09-28T12:00:00Z"
        request["options"][0]["metrics"]["cost"] = ["a/cost"]
        packet = make_packet(request)
        self.assertEqual(self.option(packet)["status"], "feasible")
        states = {r["id"]: r["state"] for r in packet["evidence"]}
        self.assertEqual(states["a/cost-v2"], "future")

    def test_revision_changes_result_without_erasing_old_source(self):
        packet = make_packet(cases()["revised"])
        self.assertEqual(self.option(packet)["status"], "infeasible")
        states = {r["id"]: r["state"] for r in packet["evidence"]}
        self.assertEqual(states["a/cost"], "superseded")
        self.assertEqual(packet["comparison"]["pareto_frontier"], ["c"])

    def test_memory_loses_support_when_its_fact_is_superseded(self):
        request = scenario()
        request["evidence"].append(evidence("budget-v2", "limits/budget", "launch", "cost", 500, "USD",
                                            supersedes=["budget"], observed_at="2026-09-29T00:00:00Z"))
        packet = make_packet(request)
        self.assertEqual(packet["memory"][0]["state"], "unsupported")
        self.assertEqual(self.option(packet)["status"], "unknown")

    def test_supersession_cycles_are_rejected(self):
        request = scenario()
        request["evidence"][0]["supersedes"] = ["budget"]
        with self.assertRaises(DecisionError) as caught:
            validate(request)
        self.assertEqual(caught.exception.code, "input.supersession_cycle")

    def test_cache_and_persisted_identity_follow_content(self):
        with tempfile.TemporaryDirectory() as folder:
            state = State(Path(folder) / "state.sqlite")
            try:
                request = scenario()
                merged = state.merge(request)
                validate(merged)
                state.ingest(request["evidence"], request["memory"])
                key, cached = state.cached(merged)
                self.assertIsNone(cached)
                packet = make_packet(merged)
                state.save(key, packet)
                self.assertEqual(state.cached(merged)[1], packet)
                changed = cases()["revised"]
                new_snapshot = state.merge(changed)
                self.assertIsNone(state.cached(new_snapshot)[1])
                changed_time = dict(merged, as_of="2026-09-30T00:00:00Z")
                self.assertIsNone(state.cached(changed_time)[1])
                changed = copy.deepcopy(request)
                changed["evidence"][0]["fact"]["value"] = 50
                with self.assertRaises(DecisionError) as caught:
                    state.merge(changed)
                self.assertEqual(caught.exception.code, "memory.identity_conflict")
            finally:
                state.close()
            reopened = State(Path(folder) / "state.sqlite")
            try:
                empty = dict(request, evidence=[], memory=[])
                self.assertEqual(reopened.merge(empty)["evidence"], merged["evidence"])
            finally:
                reopened.close()


if __name__ == "__main__":
    unittest.main()
