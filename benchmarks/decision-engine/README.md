# Reasonix decision packet prototype

An offline, CPU-only decision computation core. Given explicit facts, sources,
options, constraints and preference directions, it persists evidence and decision
notes, checks feasibility and projects a cited decision packet for a speaking
model. An optional local GPU pipeline now interprets questions and attempts
explanations using the cached DeepSeek backbone. Neither mode calls paid providers.
It uses no phrase tables or classification head. The prior
task-kind experiment does not supply authority or relevance for this prototype.

## Current behavior

- SQLite preserves imported evidence and decision-note identities. Changing an
  existing identity is rejected; updates require a new ID and explicit
  `supersedes` references. Old records remain available with their state.
- Records include source URI/version, observation time and optional expiration.
  Expired, future, conflicting and superseded facts cannot satisfy constraints.
  Notes with invalid supporting evidence are marked unsupported.
- Evidence selection follows declared topic identities and all option metric,
  constraint and selected-note references. Conflicting counterparts are retained.
  Source text is preserved verbatim. No model discovers relevant topics yet.
- Numeric comparisons use Decimal and explicit matching units. Missing data is
  unknown, never a passing constraint. Budget limits must match their cited fact.
  Feasibility is conditional on imported evidence; sources are not independently
  verified or fetched.
- Pareto comparison uses only declared preference directions and known-feasible
  options. It preserves tradeoffs and never supplies utility weights or a final
  recommendation. Unknown options may change the frontier.
- Cached packets bind to all canonical input content, engine version and exact
  `as_of` time. Changes invalidate reuse. This is full-snapshot caching, not
  partial recomputation or a provider prompt-cache savings claim.
- All outputs are advisory. This is a local experiment, with no integration into
  Reasonix's controller, decision provider, permissions or commercial service.

## Run

From the repository root, the engine itself needs only standard-library Python:

```powershell
python benchmarks/decision-engine/demo.py --output temp/decision-engine/fixtures
python benchmarks/decision-engine/run.py temp/decision-engine/fixtures/normal.json --state temp/decision-engine/normal.sqlite --output temp/decision-engine/packet.json
python -m unittest discover -s benchmarks/decision-engine -p 'test_*.py'
```

For exact counts with the already cached DeepSeek tokenizer:

```powershell
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-engine/run.py temp/decision-engine/fixtures/normal.json --state temp/decision-engine/normal.sqlite --output temp/decision-engine/packet.json --tokenizer-cache temp/decision-model/model-cache
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-engine/evaluate.py --tokenizer-cache temp/decision-model/model-cache --output temp/decision-engine/measured.json
```

The tokenizer is local-only and pinned to the existing experiment's revision.
No neural inference or GPU work is needed. Without it, token fields remain null;
byte counts are not substituted for token counts. The default packet budget is
4096 tokens; overflow is reported rather than truncating mandatory evidence.
Repeating the first command with identical state and input demonstrates a cache
hit. Use a separate SQLite file for each project/trust domain. The CLI does not
provide multi-user isolation or authorization.

## Input and persistence contract

`example-request.json` is a compact authored example. `evidence` and `memory`
records are imported into the selected state file. Incoming records merge with
stored records before validation, allowing an update to reference an older ID.
Invalid snapshots are rejected before ingestion. Omitting a record in a later
input does not delete it; supersession and time validity determine its state.
Question, topics, options, constraints and preferences come from the current
request and are not inferred from source wording. Unknown schema references
remain explicit errors or missing-evidence entries, not guessed values.

Fact records declare `metric`, `value`, `unit` and a stable semantic `key`.
Concurrent current values for the same key conflict. Sources with different
spellings of the same semantic key are not automatically reconciled. An upstream
adapter must validate identity assignments and fact extraction before deployment.
Unit conversion, currencies, probabilistic forecasts and sensitivity analysis are
not implemented. An imported score is an assumption, not a measured outcome.

Error responses carry dotted codes such as `input.supersession_cycle` and
`memory.identity_conflict`. Source content is untrusted data, not executable code
or instructions. The prompt projection uses the same evidence-use instruction
for raw context and decision packets. That instruction is not a security sandbox.

## Measured pilot

`measured-rtx3060-host.json` records seven authored variants of one planning
scenario, tokenizer identity, measurement boundaries and exclusions. Normal
inputs contain 24 deliberately irrelevant, repetitive archive records; those
are omitted by explicit topic/reference selection.

| Authored fixture | Raw input tokens | Full packet tokens | Change |
| --- | --- | --- | --- |
| Long context with unrelated archive | 7,394 | 1,960 | 73.5% fewer |
| Already compact context | 1,312 | 1,848 | 40.9% more |

The other fixtures exercise expired evidence, conflicts, missing sources,
incompatible units and a revised cost that changes feasibility. Referenced option
and constraint sources are either retained or explicitly flagged missing; none
silently disappear. Seven fixture checks pass, but they verify declared mechanism
behavior, not user decision quality. The unit tests additionally exercise memory
support, persistence, cache invalidation and invalid supersession.

Token counts cover canonical raw context versus the full packet with the same
instruction; they omit chat templates, natural-language extraction, provider
inference, output tokens, retries and total cost. Timing starts after fixture
loading, validation, ingestion and tokenizer loading. One run per case does not
establish a latency distribution. No actual customer savings or faster model
response is established. Small inputs can expand, and future integration must
account for the entire pipeline rather than forwarding packets unconditionally.

## Next integration boundary

Add a validated adapter from the model's structured interpretation or user form
to this contract. Preserve source identities through Reasonix's canonical memory
and tools, then project the packet at the request tail through the shared
controller. Do not mutate the cache-stable system prefix or turn this advisory
packet into a permission gate. Before that integration, evaluate unseen decisions
with and without packets, including evidence omissions, constraint violations,
output tokens, retries, total latency and cost. Neither existing classification
checkpoint is a substitute for this evaluation.

## Natural-language pipeline pilot

`pipeline.py` connects a speaking-model interface to the existing computation
core. It accepts a natural-language question within a supplied structured
catalog. Interpretation returns existing topic IDs, focus option IDs and typed
needs-information codes. It cannot invent facts, constraints, weights or new
options. Focus options annotate the question; the engine still checks every
catalog option and preserves all host-declared constraints and preferences.
Questions outside the catalog need additional data; arbitrary prose-to-fact
extraction is not implemented.

Only validated interpretations proceed to computation and state ingestion.
Evidence and memory are still imported from the catalog, never invented by the
model. Explanation output must contain the requested schema, every option and
existing source identities supporting each option. These checks do not verify
the truth of every prose sentence. Accepted explanations still require review;
rejected explanations remain recorded in diagnostic traces and are not presented
as validated answers. The computed packet remains separately available when
explanation generation fails.

The local model uses greedy generation with its reasoning prefix explicitly
closed and a JSON opening prefilled in the input. It loads pinned official
safetensors from the existing cache, with remote code and downloads disabled.
This runtime uses the GPU and the full frozen language-model backbone; the
classifier checkpoints do not participate. It is a protocol experiment, not
fine-tuning or proof of reliable model semantics.

```powershell
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-engine/pipeline.py benchmarks/decision-engine/example-request.json --question 'Compare the product improvements under our existing budget and deadline. Show the cost and impact tradeoffs.' --state temp/decision-engine/pipeline-demo.sqlite --output temp/decision-engine/pipeline-demo.json --model-cache temp/decision-model/model-cache --compare --attempts 1
```

The pipeline compares tokenizer-counted packet and raw-context-with-checks
projections, choosing the smaller without discarding mandatory checks. Token
budgets apply to interpretation, explanation, baseline and repair calls; required
content is never silently truncated. This choice does not account for model
answer-quality differences or eliminate the cost of the interpretation call.
The direct baseline has the same question and response contract, receives the
raw context, and performs no engine computation or interpretation. Reported
pipeline totals include all interpretation/explanation attempts and repairs;
baseline totals include all direct-answer attempts and repairs. Both use the
same local model and generation settings in each paired run. Money saved and
answer-quality equivalence remain null rather than inferred from token counts.

`local-pipeline-pilot.json` preserves actual local runs and failures. The initial
plain generation failed interpretation twice. JSON-prefilled generation produced
a schema-valid interpretation for the authored English question; explanation
generation remained unreliable despite a bounded response-shape example. The
example is developmental, not an unseen benchmark. No accepted local explanation
or production-readiness result is established. Short inputs can cost more once
interpretation and retries are counted. See the pilot report for per-run counts.

Scripted tests verify full accounting, mandatory-constraint preservation,
invalid-source rejection, needs-information states, budget handling and failure
without ingestion. They establish implementation behavior under supplied model
responses, not actual model accuracy. The real GPU traces are distinct from
these scripted tests. The native integration below has its own effect test;
the Python natural-language pipeline remains an independent experiment.

## Native Reasonix comparison capability

`internal/tools/decision` implements `tool:decision_compare` in Go, registered
through the shared boot runtime catalog. The main conversation model discovers its
schema with `use_capability` inspect and submits sourced numeric facts through
`use_capability` call. There is no separate interpretation-model invocation,
Python process, GPU dependency or network request inside the tool.

`native-request.json` is an authored two-plan example: A satisfies the budget;
B exceeds it. Use its object as `arguments` in
`{"action":"call","capability_id":"tool:decision_compare","arguments":{...}}`.
Inspect first with
`{"action":"inspect","capability_id":"tool:decision_compare"}`.

The tool accepts decimal strings, explicit constraints, preference directions,
source identities, observation times and optional expiry/supersession links.
It uses exact rational arithmetic, reports missing, stale and conflicting
evidence, preserves the caller's constraints, and returns the Pareto frontier
over known-feasible options. It never selects a final winner. Evidence keys
identify the same subject and metric across revisions; different options must
use different keys. Units must match exactly; unit conversion is not inferred.

Source contents remain caller-supplied and unverified. The main model can omit
or misrepresent facts or constraints before the call; this tool cannot establish
that its input covers the user's full question. It is stateless and does not
import the Python prototype's SQLite decision memory or natural-language
interpretation. Non-numeric judgments and arbitrary document extraction remain
outside this capability.

`TestEffectDecisionCompareReachesProviderWithoutExtraInference` runs real
`boot.Build`, discovers the schema, and checks that computed and expired-evidence
results reach subsequent provider requests. It verifies the provider tool schema
stays stable and that these scripted tool calls add no separate inference call.
These fixtures establish plumbing and arithmetic behavior, not model accuracy,
real-world answer quality, end-to-end speed or token savings. Those require
paired evaluation with a real speaking model and unseen tasks.

```powershell
go test ./internal/tools/decision
go test ./internal/assembly/boot -run TestEffectDecisionCompare -count=1
go test ./internal/tools/decision -run '^$' -bench BenchmarkDecisionCompare -benchmem
```

## Native evaluation findings

The Go evaluation adds 1,000 generated one-metric comparisons across `lte`,
`gte`, `eq`, negative values and both preference directions, plus 300 generated
three-metric tradeoffs. Independent integer comparisons determine the expected
constraint results and frontier. All 1,300 generated cases pass. These are
synthetic arithmetic checks, not a model accuracy benchmark. Statement coverage
for the native package is 87.0% in the local run.

Adversarial cases reproduced three ambiguous comparison states: all options
violating constraints, no known-feasible options, and no supplied preferences
were all reported as `insufficient_information`. They now have distinct states:
`no_feasible_options`, `no_known_feasible_options`, and
`preferences_not_provided`. Unknown constraint checks carry value/limit
diagnostics, including missing metrics and limit/unit mismatches. Known
diagnostics are omitted to avoid repeating the resolved values. The real boot
effect test confirms expiry diagnostics reach the subsequent provider request.

`native_model_eval.py` compares raw structured requests with the native tool's
complete result on four authored cases: budget, expired price, no feasible plan,
and conflicting prices. Both arms use the same cached 1.5B model, Chinese output
contract, greedy decoding and 192-token output ceiling, without retries. Exact
decision match requires the entire four-field response to equal the expected
object; correct partial fields never count as a passing answer. Duplicate JSON
fields, invented winners and missing options are rejected by the evaluator.

`native-model-evaluation.json` preserves the initial protocol: 0/4 exact matches
in each arm, mostly copying input to the output ceiling. Its no-feasible raw
fixture retained the old source prose while changing the numeric fact; do not
use that initial run to judge prose/fact reasoning. `native-model-task-evaluation.json`
uses a direct user instruction and a schema-specific response prefix, with
consistent source prose; exact matches remain 0/4. The final result after
omitting known diagnostic duplication is in `native-model-final-evaluation.json`:
raw 0/4, native 1/4. The native expired-price response has correct option states
but fails the full output contract. Conflicting and all-infeasible responses
remain unreliable. These tiny development samples do not establish population
accuracy or attribution to any single change.

In the final four paired explanation calls, native inputs total 3,251 tokenizer
tokens versus raw 2,259 (44% more); generated outputs total 490 versus 228.
This counts the complete chat template, input prefill and generated EOS; it
excludes model loading, actual agent discovery/call generation, computation and
retries. Failed answers are included. No equivalent-quality cost or latency
saving is established. On this i5-14600K, local tool benchmarks measured about
64 microseconds for the two-option fixture and 2.52 milliseconds for 32 options
with three metrics, including decoding, validation, comparison and JSON output;
they exclude model calls and tool dispatch. The 32-option benchmark allocates
about 1.52 MB per operation, so server concurrency still needs measurement.

Priorities supported by these runs are: reuse the existing main speaking model;
validate its answer before presenting it; expose the computed result directly
when explanation fails; reduce repeated source/diagnostic context while keeping
constraints and uncertainty visible; then evaluate unseen tasks and repeated
evidence use. Arbitrary prose extraction, actual
source verification and real-main-model agent evaluation remain unfinished.

```powershell
go build -o temp/decision-engine/native-eval.exe ./benchmarks/decision-engine/native-eval
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-engine/native_model_eval.py --runner temp/decision-engine/native-eval.exe --model-cache temp/decision-model/model-cache --output temp/decision-engine/native-evaluation.json --protocol task
```

## Structured answer checking and fallback

The stateless experimental constructor `decision.NewCheck()` accepts `request`
(the original comparison input) and `candidate_json` (the model's unmodified
structured answer text). It recomputes the supplied request, checks exact option identities/statuses, the frontier as a
set, comparison state, and a null recommendation. The candidate must have exactly
`options`, `frontier`, `comparison_state`, and `recommended_option`. Unknown
fields, duplicate JSON keys, missing fields, malformed/truncated JSON, missing
options, invented options or winners, and incorrect judgments are rejected.

The result always returns a canonical `decision` and sourced `basis` for valid
requests. `answer_accepted` means the candidate's structured claims agree;
`computed_fallback` includes typed reasons and returns the computed answer
instead. Known source uncertainty, constraints and expiry/conflict diagnostics
remain in the basis. Invalid original requests return `invalid`, since they do
not support a trustworthy computed fallback. There is no repair model call.

Both tools are optional runtime capabilities assembled by shared `boot.Build`,
not members of the static builtin inventory. They respect a non-empty enabled
tool list and are inspected through `use_capability`. Static builtin schema and
provider-visible schema budgets stay unchanged. Boot effect tests verify schema
discovery, valid acceptance, semantic rejection, malformed-candidate fallback,
source/scope preservation, and stable provider tool schemas.

`native-answer-check-evaluation.json` replays the eight previously recorded local
model answers against their original host-recorded inputs: 1 accepted, 7 replaced
with computed fallback, and all 8 canonical deliveries agree with the recorded
computed expectations. This measures consistency and fallback behavior on those
fixtures, not improved model accuracy, real-user quality or source truth.

Stateless experiment callers must retain and supply the original input. A model
can change a request before submitting it; that constructor does not bind it to
an earlier comparison snapshot. It neither validates arbitrary prose nor intercepts the
ordinary final chat message. Frontends may present returned structured decisions;
automatic mandatory checking of all final decisions still needs a host-owned
decision workflow. Replay preserves the local runtime's generation finish reason
separately; the checker does not infer output-limit attribution from malformed
text. Source verification remains separate work.

```powershell
go build -o temp/decision-engine/native-eval.exe ./benchmarks/decision-engine/native-eval
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-engine/check_model_answers.py --runner temp/decision-engine/native-eval.exe --recorded benchmarks/decision-engine/native-model-final-evaluation.json --output temp/decision-engine/answer-check.json
```

## Runtime request snapshots

The in-memory experiment constructor `decision.NewRuntime()` exposes comparison,
checking and evidence updates with one private snapshot store. Runtime `decision_compare`
returns `snapshot_id` and `snapshot_expires_at` alongside its computed result.
Runtime `decision_check` accepts only `snapshot_id` and `candidate_json`; it
loads the immutable original request and recomputes it. Request replacement is
rejected. Its validation scope is
`structured_consistency_with_host_snapshot_only`. Stateless constructors remain
available to benchmark programs but are not registered in the runtime catalog.

Each independently built runtime owns its store. Snapshots use random 128-bit
IDs, live for 30 minutes, and retain the original `as_of` reference time; they
do not fetch new facts or turn historical evidence into present-day evidence.
Identical canonical requests reuse their ID without renewing expiry. The store
holds at most 64 snapshots of 256 KiB canonical serialized input each (16 MiB
of retained serialized input, excluding Go overhead). Capacity/size failures
are explicit; no live snapshot is silently evicted or truncated. Expired rows
are reclaimed when saving another request. Unknown or expired IDs return typed
errors without a guessed fallback. The cache writes no filesystem state.

Snapshots disappear with the owning runtime and do not survive restart or
rebuild. A controller's branches may share a runtime; this is independent-runtime
isolation, not a multi-tenant authorization guarantee. The initial submitted
facts and constraints remain caller-supplied and unverified. Structured checks
are callable capabilities, not mandatory interception of free-form final text.

`native-snapshot-evaluation.json` replays the same eight recorded answers in one
live native process: 1 accepted, 7 computed fallbacks, all 8 canonical decisions
match the recorded expectations and all bind to the saved request's SHA-256.
Using the pinned cached tokenizer, serialized check arguments total 3,973 tokens
with full-request resubmission versus 1,047 using snapshot IDs (about 74% fewer).
The metric counts only check arguments and excludes initial comparison inputs,
inspection schemas, response bodies, chat history, output tokens and retries.
It establishes reduced retransmission, not whole-conversation or money savings.
No new model calls participate in this replay.

Unit tests cover immutable copies, duplicate-request reuse, replacement rejection,
cross-runtime lookup rejection, capacity and exact expiry boundaries, canonical
byte limits and concurrent access. The concurrent test passes Go's race detector.
Expiry tests advance an injected clock; they do not claim a 30-minute live wait.
Real boot effect tests obtain the ID from a comparison tool result and use it in
subsequent check calls, asserting sourced fallback and a stable provider surface.

```powershell
go build -o temp/decision-engine/native-eval.exe ./benchmarks/decision-engine/native-eval
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-engine/replay_snapshot_answers.py --runner temp/decision-engine/native-eval.exe --recorded benchmarks/decision-engine/native-model-final-evaluation.json --model-cache temp/decision-model/model-cache --output temp/decision-engine/snapshot-evaluation.json
```

## Durable snapshots and explicit evidence versions

Production `boot.Build` registers `decision.NewPersistentRuntime()` with a
host-selected SQLite file under session storage, namespaced by workspace path.
Rebuilding a controller with the same storage root and workspace restores IDs.
Controllers in that scope share records; other workspace paths use separate
files. This is account-local persistence, not tenant authentication or access
control. Moving a workspace or changing session storage does not migrate IDs.

Persistent records expire seven days after creation and retain their original
`as_of` reference time. Each workspace holds at most 256 live records, each with
at most 256 KiB of canonical request bytes (64 MiB of live serialized inputs,
excluding database overhead, reusable pages and journals). Retry reuses the
same ID without extending expiry. Saving reclaims expired rows; retention is
logical expiry, not secure deletion or a fixed disk-file size limit.

`decision_update` requires a parent `snapshot_id`, `as_of`, appended `evidence`
with new IDs, and explicit `metric_updates`. Source revisions can declare
`supersedes`; existing evidence cannot be overwritten. Option identities,
constraints and preferences remain fixed. A successful update returns a new
`snapshot_id` and `parent_snapshot_id`. Checks against the parent continue using
the original evidence; a stale parent answer can fail against the child.
Expired or missing parents cannot create new versions. Additional evidence and
metric changes still pass all original input limits and validations.

Writes use SQLite transactions, bounded busy waiting and full synchronous
commit. Compare/update are classified as writes by the host; checking opens the
database read-only. Unsupported databases, write failures and inconsistent
records return typed errors without falling back to volatile memory. Request
hashes and record checksums detect inconsistent data, not authenticated source
truth or a malicious writer who rewrites both data and checksum.

`native-persistence-evaluation.json` records one authored two-option fixture
across three separate native processes: comparison, restore/update, then checks
of both versions. All six assertions pass, including unchanged parent results,
linked new version, rejection of an old answer on the new version, and acceptance
of the new answer. This tests restart persistence and structured consistency;
there are no model calls or new accuracy/cost claims. Real `boot.Build` effect
tests also close and rebuild controllers twice, obtain IDs from actual provider
tool results, and check both versions with stable provider schemas.

Unit tests cover concurrent deduplication, seven-day expiry with an injected
clock, capacity, immutable constraints, update validation, read-only missing
stores, unsupported versions and externally altered rows. Corruption fixtures
use direct test SQL writes; they demonstrate detection given inconsistent state,
not an observed production corruption event or a malicious-tampering boundary.

```powershell
go build -o temp/decision-engine/native-eval.exe ./benchmarks/decision-engine/native-eval
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-engine/replay_persistence.py --runner temp/decision-engine/native-eval.exe --output temp/decision-engine/persistence-evaluation.json
```

## Checked reports without a separate explanation model

The optional runtime capability `tool:decision_deliver` takes the same
`snapshot_id` and `candidate_json` as `decision_check`. It loads the original
version, computes and validates the answer, then returns the canonical decision
and a deterministic Chinese report. Wrong, malformed or prose candidates produce
`computed_fallback`; their text never enters the report. Missing, expired or
corrupt snapshots produce typed errors with no deliverable report.

The report includes reference time, snapshot expiry and parent version, original
constraints and preference directions, option metrics, constraint verdicts,
unknown-evidence causes, source versions and supersession links. It keeps source
uncertainty and the conditional frontier visible, without naming a unique winner.
The response carries `snapshot_sha256` and the validation scope separately.
There is no explanation model call, network access or write. Unlike the diagnostic
checker response, it does not repeat the full machine-readable evidence basis.

`report.media_type` is `text/plain`; consuming interfaces must display
`report.text` as plain text, treating quoted names and source metadata as
untrusted data. This is a callable report capability, not an automatic frontend
card or interception of ordinary final chat. The current renderer is Chinese;
localized report selection and frontend-specific presentation remain separate
work. Output larger than 256 KiB of serialized response is refused with
`decision.report_output_limit` rather than returned as a partial report.

`native-delivery-evaluation.json` replays eight previously recorded local-model
answers across four authored cases: one accepted, seven computed fallbacks, all
eight canonical results bound to their saved request hashes. The two candidates
in each case yield identical report text. Report generation invokes no model.
This proves the local report path and consistency on those fixtures; it does not
measure real-user quality, end-to-end speed, total tokens or monetary savings.
`native-delivery-example.txt` contains a generated report from the first case.
Unit tests cover uncertainty, literal field boundaries, explicit output limits
and restored child-version reports. A real `boot.Build` effect test discovers
and calls the capability, verifies the report reaches the provider with corrected
claims, and confirms the provider's static tool prefix remains unchanged.

```powershell
go build -o temp/decision-engine/native-eval.exe ./benchmarks/decision-engine/native-eval
& ./temp/decision-model/venv/Scripts/python.exe benchmarks/decision-engine/replay_delivery.py --runner temp/decision-engine/native-eval.exe --recorded benchmarks/decision-engine/native-model-final-evaluation.json --output temp/decision-engine/delivery-evaluation.json --example temp/decision-engine/delivery-example.txt
```

## Authenticated HTTP report delivery

`POST /decision/deliver` now accepts `{snapshot_id, candidate_json}` and returns
the same checked canonical decision and plain-text report as the native delivery
tool. The shared controller owns this operation; assembly binds it to the
enabled `decision_deliver` capability and its workspace store. A non-empty
tool allowlist excluding that capability also disables the HTTP operation.
The endpoint reads existing snapshots only. Create and update snapshots through
the existing native capabilities; this route does not accept replacement inputs,
change evidence or intercept the chat stream.

The route uses the existing server authentication, host and CSRF guards. JSON
content type and the launch credential are required even in server auth mode
`none`. Responses are marked `Cache-Control: no-store`; requests and response
reports are bounded to 256 KiB. Workspace/controller rebinding is serialized
against report generation. Existing server authentication authorizes access to
one account/workspace scope, not independent customer or tenant ownership.

Candidate rejection is a successful HTTP 200 `computed_fallback`, since a
canonical report was produced. Invalid request JSON/arguments return 400;
missing snapshots 404; expired snapshots 410; oversized inputs 413; oversized
reports 422; unavailable capabilities/storage 503. Domain failure responses carry
their producer's `decision.*` code. Storage failures do not become bad-input
responses or guessed reports. Transport cancellation and deadlines keep their
own identities. Chinese and English refusal wording are included in the frontend
catalogue; the report renderer itself remains Chinese.

Tests exercise authorized delivery, restored snapshots, incorrect candidates,
strict duplicate-field rejection, missing IDs, input limits, disabled capability,
invalid database handling, no-cache headers and missing credentials. A real
`boot.Build -> authenticated HTTP` effect test loads an authored comparison saved
through the public native comparison tool and delivers its corrected report with
zero provider inference calls. This is evidence for the report endpoint's effect,
not a paid deployment, tenant isolation, production workload or latency benchmark.

```powershell
$decisionBody = @{ snapshot_id = "<saved snapshot_id>"; candidate_json = '{}' } | ConvertTo-Json -Compress
$decisionResponse = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:8080/decision/deliver' -ContentType 'application/json' -Headers @{ Authorization = "Bearer <launch token>" } -Body $decisionBody
$decisionResponse.report.text
```
