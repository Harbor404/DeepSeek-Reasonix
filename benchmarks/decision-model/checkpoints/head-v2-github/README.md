# GitHub classification candidate v2 — not promoted

This 123,184-byte linear head requires the separate frozen DeepSeek-R1-Distill-
Qwen-1.5B backbone. It is an experimental task-kind classifier, not a code-solving
language model. No backbone parameters were trained.

SHA-256: `8064c56139e355f05d0170d5a9f6abd79d1bb8f4ab6699ba59938750eb3a0fa6`.

Training used 30 reviewed GitHub issues plus 396 previous training rows. Six
Werkzeug issues selected the checkpoint; sixteen Rich, Requests and HTTPX issues
were held out. All sixteen test predictions were atomic-bugfix, matching the
majority baseline at 10/16. Existing-task accuracy regressed from 29/51 to 14/51.
Do not replace v1 with this candidate. See the experiment README and metadata
for provenance, data-rights limitations and reproduction instructions.
