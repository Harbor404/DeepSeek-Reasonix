import argparse
import json
import threading
from pathlib import Path
from urllib.request import Request, urlopen

from run import MODEL_ID, Scorer
from service import make_server


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--revision")
    parser.add_argument("--checkpoint", type=Path)
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[2]
    if args.checkpoint:
        from head_model import HeadScorer
        scorer = HeadScorer(args.checkpoint)
    else:
        if not args.revision:
            parser.error("Supply --checkpoint or --revision")
        scorer = Scorer(repo / "temp/decision-model/model-cache", args.revision)
    server = make_server(scorer, 0)
    worker = threading.Thread(target=server.serve_forever, daemon=True)
    worker.start()
    endpoint = f"http://127.0.0.1:{server.server_port}"
    rows = []
    try:
        with urlopen(endpoint + "/health", timeout=30) as response:
            health = json.load(response)
        if health["revision"] != scorer.revision:
            raise RuntimeError("Unexpected model revision")
        for text in ("Implement a function that adds two integers.",
                     "修复 pricing.py 中没有正确应用折扣的问题，保持现有函数接口不变。"):
            body = json.dumps({"text": text}, ensure_ascii=False).encode("utf-8")
            request = Request(endpoint + "/v1/task-kind", data=body,
                              headers={"Content-Type": "application/json"}, method="POST")
            with urlopen(request, timeout=60) as response:
                result = json.load(response)
            if not result["advisory"] or result["prediction"] not in health["categories"]:
                raise RuntimeError("Invalid decision response")
            rows.append({"text": text, "result": result})
    finally:
        server.shutdown()
        server.server_close()
        worker.join(timeout=3)
    report = {"model": MODEL_ID, "revision": scorer.revision,
              "head_sha256": getattr(scorer, "head_sha256", None),
              "purpose": "Real GPU API transport smoke check, not an accuracy or speed benchmark",
              "requests": rows}
    output = repo / ("temp/decision-model/trained-head-results/api-smoke.json" if args.checkpoint
                     else "temp/decision-model/results/api-smoke.json")
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
