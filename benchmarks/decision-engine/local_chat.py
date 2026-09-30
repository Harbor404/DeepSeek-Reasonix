import sys
import time
from pathlib import Path

from contract import DecisionError


class LocalChat:
    def __init__(self, cache, max_output_tokens=512, response_prefix='{"'):
        import torch
        from transformers import AutoModelForCausalLM, AutoTokenizer
        sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "decision-model"))
        from model_config import MODEL_ID, REVISION
        if not torch.cuda.is_available():
            raise DecisionError("model.cuda_unavailable")
        self.torch, self.model_id, self.revision = torch, MODEL_ID, REVISION
        self.max_output_tokens = max_output_tokens
        self.response_prefix = response_prefix
        self.tokenizer = AutoTokenizer.from_pretrained(
            MODEL_ID, revision=REVISION, cache_dir=str(cache), local_files_only=True, trust_remote_code=False)
        self.model = AutoModelForCausalLM.from_pretrained(
            MODEL_ID, revision=REVISION, cache_dir=str(cache), local_files_only=True,
            trust_remote_code=False, use_safetensors=True, dtype=torch.float16,
            attn_implementation="sdpa").to("cuda").eval()
        self.model.requires_grad_(False)

    def encode(self, messages):
        rendered = self.tokenizer.apply_chat_template(messages, tokenize=False, add_generation_prompt=True)
        return self.tokenizer.encode(rendered + '</think>\n' + self.response_prefix, add_special_tokens=False)

    def count(self, messages):
        return len(self.encode(messages))

    def complete(self, messages):
        started = time.perf_counter()
        ids = self.encode(messages)
        if len(ids) + self.max_output_tokens > self.model.config.max_position_embeddings:
            raise DecisionError("model.context_limit")
        torch = self.torch
        inputs = torch.tensor([ids], device="cuda")
        with torch.inference_mode():
            output = self.model.generate(
                input_ids=inputs, attention_mask=torch.ones_like(inputs),
                do_sample=False, max_new_tokens=self.max_output_tokens,
                pad_token_id=self.tokenizer.eos_token_id)
        generated = output[0, len(ids):].tolist()
        torch.cuda.synchronize()
        ended = bool(generated and generated[-1] == self.tokenizer.eos_token_id)
        return {"text": self.response_prefix + self.tokenizer.decode(generated, skip_special_tokens=True),
                "usage": {"input_tokens": len(ids), "output_tokens": len(generated)},
                "finish_reason": "eos" if ended else "output_limit",
                "latency_seconds": time.perf_counter() - started,
                "model": self.model_id, "revision": self.revision,
                "protocol": "Greedy JSON-prefilled generation with reasoning prefix explicitly closed; local counts include chat template, input prefill and generated EOS",
                "response_prefix": self.response_prefix}
