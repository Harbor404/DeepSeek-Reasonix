import json
import unittest

from native_model_eval import evaluate_response


class EvaluationTests(unittest.TestCase):
    def setUp(self):
        self.expected = {"options": {"a": "unknown", "b": "infeasible"}, "frontier": [],
                         "comparison_state": "no_known_feasible_options", "recommended_option": None}

    def test_partial_agreement_is_not_an_accepted_answer(self):
        response = {**self.expected, "frontier": {}, "evidence": {}}
        got = evaluate_response(json.dumps(response), self.expected)
        self.assertTrue(got["option_statuses_match"])
        self.assertFalse(got["exact_decision_match"])

    def test_duplicate_fields_are_not_accepted(self):
        text = json.dumps(self.expected).replace('"recommended_option": null', '"recommended_option": "a", "recommended_option": null')
        self.assertFalse(evaluate_response(text, self.expected)["valid_json"])

    def test_infeasible_winner_and_missing_options_are_rejected(self):
        for response in [{**self.expected, "recommended_option": "b"},
                         {**self.expected, "options": {"a": "unknown"}}]:
            self.assertFalse(evaluate_response(json.dumps(response), self.expected)["exact_decision_match"])


if __name__ == "__main__":
    unittest.main()
