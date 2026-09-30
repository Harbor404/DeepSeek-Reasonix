from pathlib import Path

from model_config import FEATURE_SCHEMA, MODEL_ID, REVISION


class InputTokenLimit(ValueError):
    def __init__(self, counts):
        self.counts = counts
        super().__init__("Input exceeds encoder token limit")


class FrozenEncoder:
    def __init__(self, cache, revision=REVISION, max_tokens=1024):
        import torch
        from transformers import AutoModel, AutoTokenizer

        if not torch.cuda.is_available():
            raise RuntimeError("CUDA is required; no silent CPU fallback")
        self.torch = torch
        self.revision = revision
        self.max_tokens = max_tokens
        self.feature_schema = FEATURE_SCHEMA
        self.tokenizer = AutoTokenizer.from_pretrained(
            MODEL_ID, revision=revision, cache_dir=str(cache), local_files_only=True,
            trust_remote_code=False)
        self.tokenizer.pad_token = self.tokenizer.eos_token
        self.tokenizer.padding_side = "left"
        self.model = AutoModel.from_pretrained(
            MODEL_ID, revision=revision, cache_dir=str(cache), local_files_only=True,
            trust_remote_code=False, use_safetensors=True, dtype=torch.float16,
            attn_implementation="sdpa").to("cuda").eval()
        self.model.requires_grad_(False)
        self.dimension = self.model.config.hidden_size * 2

    def encode(self, texts):
        torch = self.torch
        inputs = self.tokenizer([text + self.tokenizer.eos_token for text in texts],
                                add_special_tokens=False, padding=True, return_tensors="pt")
        counts = inputs["attention_mask"].sum(dim=1).tolist()
        if max(counts) > self.max_tokens:
            raise InputTokenLimit(counts)
        inputs = {key: value.to("cuda") for key, value in inputs.items()}
        with torch.inference_mode():
            hidden = self.model(**inputs, use_cache=False).last_hidden_state.float()
            mask = inputs["attention_mask"].unsqueeze(-1).float()
            mean = (hidden * mask).sum(dim=1) / mask.sum(dim=1)
            features = torch.cat((mean, hidden[:, -1]), dim=1)
        return features.cpu(), counts

    def encode_rows(self, rows, batch_size=8):
        features, counts = [], []
        for start in range(0, len(rows), batch_size):
            group = rows[start:start + batch_size]
            encoded, lengths = self.encode([row["prompt"] for row in group])
            features.append(encoded)
            counts.extend(lengths)
            if start % (batch_size * 8) == 0 or start + batch_size >= len(rows):
                print(f"Encoded {min(start + batch_size, len(rows))}/{len(rows)}", flush=True)
        return self.torch.cat(features).clone(), counts


def default_cache():
    return Path(__file__).resolve().parents[2] / "temp/decision-model/model-cache"
