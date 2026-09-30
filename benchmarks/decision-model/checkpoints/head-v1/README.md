# Task-kind head v1

`head.safetensors` contains a linear classifier and training-set normalization
statistics. It is not a standalone language model. Use it through the experiment's
`HeadScorer` with the frozen DeepSeek-R1-Distill-Qwen-1.5B backbone pinned to
`ad9f0ae0864d7fbcd1cd905e3c6c5b069cc8b562`.

SHA-256: `2af5484aa19f7766cbc205ffaf67d2aa1dc26bf1f0069da225daac9b66ee7094`.
The loader checks this hash, the feature schema, shapes and category mapping
before inference. `metadata.json` records training provenance and limitations.
The backbone remains a separate dependency with its applicable DeepSeek and
Qwen license notices; retain those when distributing backbone weights.

This checkpoint predicts eight advisory task kinds. It does not generate code,
grant permissions, select a paid provider or establish customer cost savings.
See the parent experiment README for measured performance and reproduction.
