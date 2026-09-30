import unittest

from run import Scorer


class TokenList(list):
    def tolist(self):
        return list(self)


class Tokenizer:
    eos_token_id = 0
    eos_token = "<eos>"

    def decode(self, ids, skip_special_tokens=False):
        fragments = {0: "<eos>", 1: "thought</think>\n\nA",
                     2: "thought</think>\n\nAnswer: A", 3: "unfinished thought"}
        return "".join(fragments[index] for index in ids)


class AnswerTests(unittest.TestCase):
    def setUp(self):
        self.scorer = Scorer.__new__(Scorer)
        self.scorer.tokenizer = Tokenizer()
        self.scorer.letters = list("ABCDEFGH")
        self.scorer.max_new_tokens = 2
        self.labels = [f"label-{index}" for index in range(8)]

    def test_batch_padding_is_not_part_of_answer_or_output_tokens(self):
        result = self.scorer.decode_answer(TokenList([1, 0, 0, 0]), self.labels, 10)
        self.assertEqual(result["prediction"], "label-0")
        self.assertEqual(result["output_tokens"], 2)

    def test_eos_at_budget_is_malformed_not_truncated(self):
        result = self.scorer.decode_answer(TokenList([2, 0]), self.labels, 10)
        self.assertIsNone(result["prediction"])
        self.assertEqual(result["reason"], "invalid_answer_schema")

    def test_unfinished_thinking_at_limit_is_a_truncation(self):
        result = self.scorer.decode_answer(TokenList([3, 3]), self.labels, 10)
        self.assertEqual(result["reason"], "output_token_limit")


if __name__ == "__main__":
    unittest.main()
