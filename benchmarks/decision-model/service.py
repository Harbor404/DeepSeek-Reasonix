import json
from http.server import BaseHTTPRequestHandler, HTTPServer

from corpus import CLASSES


def make_server(scorer, port):
    class Handler(BaseHTTPRequestHandler):
        def setup(self):
            super().setup()
            self.connection.settimeout(30)

        def reply(self, status, payload):
            body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            if self.path != "/health":
                self.reply(404, {"error": "unknown_endpoint"})
                return
            self.reply(200, {"status": "ready", "revision": scorer.revision,
                             "head_sha256": getattr(scorer, "head_sha256", None),
                             "advisory": True, "categories": CLASSES})

        def do_POST(self):
            if self.path != "/v1/task-kind":
                self.reply(404, {"error": "unknown_endpoint"})
                return
            try:
                length = int(self.headers.get("Content-Length", "0"))
            except ValueError:
                self.reply(400, {"error": "invalid_content_length"})
                return
            if not 0 < length <= 32768 or self.headers.get("Transfer-Encoding"):
                self.reply(400, {"error": "invalid_body_size_or_encoding"})
                return
            try:
                payload = json.loads(self.rfile.read(length))
            except (ValueError, TimeoutError):
                self.reply(400, {"error": "invalid_json"})
                return
            if (not isinstance(payload, dict) or set(payload) != {"text"}
                    or not isinstance(payload["text"], str) or not payload["text"].strip()):
                self.reply(400, {"error": "expected_nonempty_text_only"})
                return
            result = scorer.classify(payload["text"])
            self.reply(200 if result["prediction"] is not None else 422,
                       {"schema_version": 1, "advisory": True,
                        "revision": scorer.revision, **result})

    return HTTPServer(("127.0.0.1", port), Handler)


def serve(scorer, port):
    with make_server(scorer, port) as server:
        print(f"Advisory API: http://127.0.0.1:{server.server_port}", flush=True)
        server.serve_forever()
