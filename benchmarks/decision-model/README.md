# Local task-kind decision experiment

An offline experiment for a future Reasonix advisory capability. It scores eight
task categories with DeepSeek-R1-Distill-Qwen-1.5B on a CUDA GPU. It does not
select a production model, grant tool permissions, or change execution policy.

## First measured baseline

On this RTX 3060, the 51 included English task prompts produced:

| Method | Correct / included | Valid label responses | Timing |
| --- | --- | --- | --- |
| Forced single forward pass, batch 1 | 6 / 51 (11.8%) | 51 / 51 | Median 49.7 ms per request; 2.63 s total |
| Greedy reasoning, 512-token cap, batch 8 | 13 / 51 (25.5%) | 30 / 51 | 102.29 s total; median 14.82 s until a batch completes |
| Always choose the largest class | 11 / 51 (21.6%) | 51 / 51 | Statistical reference only |

Reasoning had 18 output-limit failures and three malformed final answers. Batch
timing is not isolated-request timing. Counts use declared task kinds as labels,
not task execution success. These baseline runs trained no model and called no
paid provider. Neither baseline establishes cost savings or production readiness.

The two `baseline-*-rtx3060.json` files preserve model and dataset revisions,
per-class recall, metric definitions, timing, and limits. Full local predictions
are in `temp/decision-model/results/` and
`temp/decision-model/reasoning-batch-results/`. An interrupted sequential
reasoning trial under `reasoning-results/` is not part of the comparison.

## Trained classification head v1

The frozen DeepSeek backbone now feeds a trained linear classifier with 24,584
learned parameters. Its eight outputs are task kinds, not answers to coding
requests. The head is 123,184 bytes, but inference still requires the full 1.5B
backbone. No backbone weights were updated.

| Evaluation population | Correct / included | Interpretation |
| --- | --- | --- |
| Authored validation | 90 / 96 (93.8%) | Used for checkpoint selection |
| Authored held-out test | 91 / 96 (94.8%) | 16 scenario families; six correlated variants each |
| Previously seen development regression | 29 / 51 (56.9%) | Same included prompts as the baselines above |

Training uses 396 rows. Translations and wrappers stay within their scenario
family and split. Exact prompt duplicates and families crossing splits are
rejected; semantic overlap is not ruled out. Labels are authored, and wrapper
styles are shared between splits. These results do not estimate customer accuracy.
The test split was scored after selection and was not used to choose the head.

The regression run took 1.57 seconds total, with median 29.0 ms and p95 45.0 ms
per sequential request, excluding loading and warmup. Peak allocated GPU memory
was 2.89 GiB. It uses plain requests with terminal EOS and a 1024-token limit,
whereas the forced baseline uses a category menu and a 2048-token limit. Timing
is for classification only. API integration scored 0/4 and read-only output 0/3;
this head is still an experimental advisory capability.

The durable checkpoint is `checkpoints/head-v1/`; its metadata records the pinned
backbone, head hash, dataset hashes and split results. The measured regression
is preserved in `baseline-trained-head-rtx3060.json`. Full local predictions are
under `temp/decision-model/trained-head-results/`.

```powershell
# Train a new checkpoint; the command refuses to overwrite an existing one.
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-model/train_head.py --output temp/decision-model/head-reproduction
# Serve the saved v1 on loopback.
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-model/run.py serve --method trained-head --checkpoint benchmarks/decision-model/checkpoints/head-v1
# Alternatively, check transport with a temporary server that closes itself.
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-model/smoke.py --checkpoint benchmarks/decision-model/checkpoints/head-v1
```

Both training and trained-head inference load the pinned backbone from the local
cache without downloading or executing remote model code. On another server,
provision that backbone in the cache before running. The linear scores remain
uncalibrated. No provider selection, successful-task cost reduction, billing or
production integration has been measured or implemented.

## GitHub issue pilot (v2 candidate, not promoted)

Authenticated read-only `gh` GraphQL access retrieved 60 merged PR records from
each of seven public Python repositories (420 PR records). A single Codex review
selected 52 linked issue reports and assigned task-kind labels with written
rationales in `github_annotations.json`. This is a convenience sample; unselected
records were not all adjudicated. Private repositories and author profiles were
not collected. No issue, PR, comment or repository was modified on GitHub.

The model sees only the issue title and current body. Linked PR metadata and
changed paths support retrospective label review and provenance, not inference.
This does not establish reproduction or fix correctness: no downloaded project
code or issue command was executed. Current issue bodies sometimes suggest fixes
and may have post-resolution edits; backbone pretraining contamination is unknown.

Repositories were assigned to splits before training: Flask, Click and pytest
provide 30 training issues; Werkzeug provides six validation issues; Rich,
Requests and HTTPX provide 16 test issues. Training retains the 396 previous
training rows for 426 total. Whole-repository separation and prompt/family audits
are enforced. Only three categories occur in the GitHub test population.

| Method | GitHub held-out issues | Existing 51-task development regression |
| --- | --- | --- |
| v1 synthetic-trained head | 1 / 16 (6.3%) | 29 / 51 (56.9%) |
| v2 GitHub candidate | 10 / 16 (62.5%) | 14 / 51 (27.5%) |
| Majority category reference | 10 / 16 (62.5%) | 11 / 51 (21.6%) |

The v2 candidate predicts atomic-bugfix for every GitHub test issue. Its balanced
accuracy over the three represented categories is 33.3%; it does not improve on
the majority reference and regresses on the existing task mix. Validation is just
six issues (five atomic bugs) and selects epoch one by cross-entropy. The dataset
and selection population are too narrow for deployment. Preserve v1; v2 is saved
only as an experimental candidate. No automatic fallback or default promotion is
implemented. Do not tune repeatedly against these now-seen 16 issues and call
them a fresh test; further experiments need a new held-out population.

`github-head-comparison-rtx3060.json` preserves the paired comparison and source
URLs. `baseline-github-head-rtx3060.json` preserves the v2 regression measurement.
`github-data-manifest-v1.json` and `github-review-ledger-v1.json` record split
hashes and reviewed sources. Raw snapshots and full selected issue text remain
in ignored `temp/decision-model/github-snapshot-v1/` and `github-data-v1/` on this
machine. The checkpoint under `checkpoints/head-v2-github/` records provenance.
Repository license identifiers are recorded; rights to issue prose have not been
independently reviewed for commercial redistribution.

```powershell
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-model/github_collect.py --output temp/decision-model/github-snapshot-reproduction
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-model/github_bundle.py --snapshot temp/decision-model/github-snapshot-reproduction --output temp/decision-model/github-data-reproduction
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-model/train_head.py --data temp/decision-model/github-data-v1 --output temp/decision-model/head-github-reproduction
```

Collection refreshes current GitHub state, so hashes may change. Exact training
reproduction uses the existing local audited `github-data-v1` snapshot. No paid
provider was called. This trains task classification, not code-solving behavior
or the choice of cheapest successful model.

## Run on the current Windows machine

```powershell
python -m venv --system-site-packages temp/decision-model/venv
& ./temp/decision-model/venv/Scripts/python.exe -m pip install -r benchmarks/decision-model/requirements.txt
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-model/run.py evaluate
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-model/run.py classify --text '修复 pricing.py 中没有正确应用折扣的问题'
& ./temp/decision-model/venv/Scripts/python.exe -m unittest discover -s benchmarks/decision-model -p 'test_*.py'
```

## Local advisory API

```powershell
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-model/run.py serve
Invoke-RestMethod http://127.0.0.1:8767/health
$requestBody = @{text = '修复 pricing.py 中没有正确应用折扣的问题'} | ConvertTo-Json
Invoke-RestMethod http://127.0.0.1:8767/v1/task-kind -Method Post -ContentType 'application/json; charset=utf-8' -Body ([System.Text.Encoding]::UTF8.GetBytes($requestBody))
```

This development API binds only to loopback and serves requests sequentially.
The body must contain only a nonempty `text` field and be at most 32 KiB. Inputs
above the model token limit return 422 rather than silently truncating. The
response is explicitly advisory and exposes no tools or execution permissions.
It has no billing, customer authentication, tenant isolation or production
scheduling. Those belong to a separately designed commercial service boundary.

After evaluating, run `smoke.py --revision <recorded SHA>` with the experiment's
Python executable to check the real GPU API with one English and one Chinese
request. It closes its temporary server and writes `api-smoke.json` alongside
the local reports. This checks transport and response structure, not accuracy.

The existing Python installation must supply a CUDA-enabled PyTorch. The
environment reuses it without modifying its packages. Model weights, the venv,
and reports live under ignored `temp/decision-model/`. Only official safetensors
weights are loaded; remote model code is disabled. The first run resolves a
model commit and records it. Use `--revision <recorded SHA>` to reproduce it.

`export` needs only Python 3.11 or newer. It derives separate training and
evaluation JSONL files from the existing training and e2e corpora. Neither
source corpus is modified. Exact whitespace-normalized cross-split duplicates
are rejected; semantic overlap is not ruled out. There are only 12 existing
training examples, insufficient for a credible training experiment.

## What the experiment measures

The target is each task's declared `class`, not which model can solve it most
cheaply. All eight supported classes are included; other classes are listed in
the manifest as excluded. Only the request text reaches the model. Task ids,
labels, reference solutions and repository contents do not.

The R1 checkpoint is forced to answer directly without generating a reasoning
trace. One forward pass scores eight single-token answer options. Category
ordering rotates deterministically by input hash. The selected label is the
largest score. Restricted softmax weights are uncalibrated; they cannot be used
as a probability of correctness or as a deployment threshold. The unrestricted
probability mass of the answer tokens is also recorded to expose forced choices.

For a normal-generation control, pass `--method reasoning --output
temp/decision-model/reasoning-results`. This leaves the thinking prefix open,
allows up to 512 new tokens, and accepts only a single letter after the
`</think>` boundary and before EOS. Other outputs count as failures. Preserve
the original single-pass report rather than overwriting it. This control is
needed to distinguish a failed forced-scoring shortcut from the model's normal
task-classification ability. It does not train or calibrate the model.

Add `--batch-size 8` to evaluate generation in groups. Each row's latency then
means time until its entire batch finishes, not isolated-request latency. Output
padding after EOS is excluded from token counts. Malformed and token-limited
answers count as incorrect and remain in latency statistics. Coverage is the
fraction with valid labels, not the fraction of inference calls attempted.

Accuracy counts agreement over all included tasks, including skipped oversized
inputs as incorrect. Balanced accuracy averages recall over represented classes.
The majority-class baseline uses the same evaluation population. Timing begins
before tokenization and ends after synchronized GPU inference and result
extraction. It excludes download, loading, warmup and task execution. Requests
run sequentially at batch size one for single-pass and trained-head methods;
generation uses the configured batch size. The report records peak allocated GPU memory,
not the total graphics memory shown by the driver.

This baseline establishes neither savings nor commercial readiness. Before
integration, collect training-only task outcomes for candidate model/workflow
choices, audit split overlap, evaluate Chinese and unfamiliar tasks, calibrate
on a separate validation split, and compare total successful-task cost and
latency against a fixed provider baseline. Predicted task kinds must remain
advisory; existing typed contracts and host permission boundaries retain authority.

## Commercial provenance

The checkpoint is a Qwen-based model fine-tuned by DeepSeek with R1 reasoning
data, not the full DeepSeek-V3 base model. DeepSeek's official R1 repository
permits commercial use and derivative work and identifies the original Qwen
Apache-2.0 license. Retain applicable notices with any distributed weights.

- [Official model repository and license notes](https://github.com/deepseek-ai/DeepSeek-R1#7-license)
- [Official checkpoint](https://huggingface.co/deepseek-ai/DeepSeek-R1-Distill-Qwen-1.5B)
