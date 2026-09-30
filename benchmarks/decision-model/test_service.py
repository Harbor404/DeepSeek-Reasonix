import json
import threading
import unittest
from urllib.error import HTTPError
from urllib.request import Request, urlopen

from service import make_server


class StubScorer:
    revision = "test-revision"

    def __init__(self):
        self.requests = []

    def classify(self, text):
        self.requests.append(text)
        return {"prediction": "codegen"}


class ServiceTests(unittest.TestCase):
    def test_invalid_input_does_not_reach_model_and_valid_unicode_is_preserved(self):
        scorer = StubScorer()
        server = make_server(scorer, 0)
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        endpoint = f"http://127.0.0.1:{server.server_port}/v1/task-kind"
        try:
            for payload in ({"text": 12}, {"text": "hello", "label": "codegen"}, []):
                request = Request(endpoint, data=json.dumps(payload).encode(), method="POST")
                with self.assertRaises(HTTPError) as caught:
                    urlopen(request, timeout=3)
                self.assertEqual(caught.exception.code, 400)
                caught.exception.close()
            self.assertEqual(scorer.requests, [])
            request = Request(endpoint, data=json.dumps({"text": "生成一个函数"}).encode(),
                              method="POST")
            with urlopen(request, timeout=3) as response:
                result = json.load(response)
            self.assertTrue(result["advisory"])
            self.assertEqual(result["prediction"], "codegen")
            self.assertEqual(scorer.requests, ["生成一个函数"])
        finally:
            server.shutdown()
            server.server_close()
            worker.join(timeout=3)


if __name__ == "__main__":
    unittest.main()
