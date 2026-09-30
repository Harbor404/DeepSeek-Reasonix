import hashlib
import json
import time
from pathlib import Path

from corpus import CLASSES
from encoder import FrozenEncoder, InputTokenLimit, default_cache
from model_config import FEATURE_SCHEMA, MODEL_ID


class HeadScorer:
    def __init__(self, checkpoint):
        from safetensors.torch import load_file

        checkpoint = Path(checkpoint)
        metadata = json.loads((checkpoint / "metadata.json").read_text(encoding="utf-8"))
        if (metadata["labels"] != list(CLASSES) or metadata["model"] != MODEL_ID
                or metadata["feature_schema"] != FEATURE_SCHEMA):
            raise ValueError("Incompatible head checkpoint contract")
        digest = hashlib.sha256((checkpoint / "head.safetensors").read_bytes()).hexdigest()
        if digest != metadata["head_sha256"]:
            raise ValueError("Head checkpoint SHA-256 mismatch")
        self.encoder = FrozenEncoder(default_cache(), metadata["revision"], metadata["max_tokens"])
        self.torch = self.encoder.torch
        self.revision = metadata["revision"]
        self.head_sha256 = digest
        self.labels = metadata["labels"]
        self.weights = load_file(str(checkpoint / "head.safetensors"))
        dimension = self.encoder.dimension
        if (self.weights["weight"].shape != (len(self.labels), dimension)
                or self.weights["bias"].shape != (len(self.labels),)
                or self.weights["mean"].shape != (dimension,)
                or self.weights["std"].shape != (dimension,)):
            raise ValueError("Invalid head tensor shapes")
        if not all(self.torch.isfinite(value).all() for value in self.weights.values()):
            raise ValueError("Nonfinite head tensor")
        if not (self.weights["std"] > 0).all():
            raise ValueError("Invalid feature normalization")

    def score_features(self, features):
        normalized = (features - self.weights["mean"]) / self.weights["std"]
        return normalized @ self.weights["weight"].T + self.weights["bias"]

    def classify(self, text):
        started = time.perf_counter()
        try:
            features, counts = self.encoder.encode([text])
        except InputTokenLimit as error:
            return {"prediction": None, "reason": "input_token_limit", "input_tokens": error.counts[0]}
        with self.torch.inference_mode():
            scores = self.score_features(features).softmax(dim=1)[0].tolist()
        self.torch.cuda.synchronize()
        return {"prediction": self.labels[max(range(len(scores)), key=scores.__getitem__)],
                "class_scores_uncalibrated": dict(zip(self.labels, scores)),
                "latency_ms": (time.perf_counter() - started) * 1000,
                "input_tokens": counts[0], "head_sha256": self.head_sha256,
                "backend": "frozen_backbone_trained_linear_head"}
