# Multi-intent interpretation comparison, 2026-10-14

Issues 27/28 now share `butler-event-interpretation-v3`: five independent intent
flags, modality and target, attached to exact source quotes. Trust/actor identity
and result verification are program state. Models return shadow observations;
ordinary catalogs, production scheduling and controller shortcuts stay disabled.

## Final independent test

All 304 classification RPCs succeed, 76 per candidate. There are 72 events and
76 source windows; the rule baseline makes no model RPC. The PostgreSQL owner-day
ledger records exactly 381 spent turns: 304 classifications plus 77 embedding
probes, 3,591,680 conservatively reserved tokens, 10,859,246 active milliseconds,
550,982 provider input bytes, zero external calls and zero billed cost. Reserved
allowance is not refunded from measured speed.

| Candidate | Exact intent set /76 | Modality /76 | Target /76 | All fields /76 | Entire events /72 |
|---|---:|---:|---:|---:|---:|
| GLiNER extractor | 7 | 54 | 25 | 0 | 0 |
| GLiNER Decide | 12 | 53 | 32 | 4 | 4 |
| d1-omni-600M | 1 | 9 | 44 | 0 | 0 |
| Qwen3.5-4B Q4_K_M | 44 | 46 | 59 | 26 | 25 |
| Fixed literal rules | 18 | 55 | 20 | 7 | 5 |

Qwen is the best tested semantic candidate, but no candidate passes the fixed
screen. The original recorded non-escalated signals are extractor5/wrong5,
Decide0, d1 0, Qwen0 and rules75/wrong68. These are shadow signals, not actual
execution or avoided controller calls. Qwen's original confidence is unknown
because the strict original scorer only recognized bare boolean tokens; the API
returns one-token ` true`/` false` with a leading space.

The separately versioned `offline-confidence-audit.json` replays the SAME saved
Qwen responses without another model call. It matches alternatives only at the
exact observed whitespace prefix, ignoring other token spellings. All 76 semantic
predictions stay unchanged; all 76 conditional distributions become available.
At the same 0.65/0.15 gates, 10 observations are non-escalated and three are wrong.
This is a post-hoc protocol audit, not a replacement independent confidence
screen or proof of calibration; the original report and frozen source remain.
The helper has its own regression check, and ordinary runtime selection stays off.

The three errors are `test-015` window1 (stop leaks from the preceding sentence
into a continuation request) and `test-065` windows0/1 (request and information
leak between a completion claim and a subsequent checking request). This exposes
a focus-isolation weakness despite correct source positions. Qwen also has one
missed direct stop, two false direct stops, three stop-scope errors, two missed
direct constraints and ten false direct-initiative observations. Predictions
never obtain owner authority or a completion verdict.

| Qwen intention | Precision | Recall |
|---|---:|---:|
| Request action | 0.636 | 0.583 |
| Change constraint | 0.769 | 0.769 |
| Stop work | 0.857 | 0.750 |
| Ask information | 0.818 | 0.750 |
| Provide information | 0.654 | 0.944 |

Literal rules score25/28 intent sets and19/28 all fields on dev, then18/76 and7/76
on the independently written final events. The design/dev score is not evidence
of broad Russian understanding. Final dev scores for extractor/Decide/d1/Qwen
are4/28,7/28,1/28,19/28 intent sets and1/28,3/28,0/28,10/28 all fields.
The dataset is small and deliberately includes hard cases; no real-world error
rate or monetary saving is inferred from these percentages.

## Placement and speed

| Candidate | First profile attempt ms | Warm p50 / p95 ms | Peak service RSS MiB |
|---|---:|---:|---:|
| GLiNER extractor | 23067 | 2190 /2278 | 3126 |
| GLiNER Decide | 23255 | 2264 /2330 | 2992 |
| d1 | 13922 | 4825 /5158 | 2284 |
| Qwen GPU | 1364 | 1342 /1361 | Not measured per request |

First CPU attempts include loading and caller overhead. Qwen was already GPU
resident from dev;1364ms is not cold loading. Each reported wall measurement
excludes the separately measured subsequent embedding probe. Seven outputs and
longer described input are a different workload from the old single-digit v2.

All77 embedding probes use accepted CPU/FP32 weights without fallback. First
probe8787ms;76 warm probes p50=82ms,p95=89ms; service peak2234MiB. The same pinned
Qwen digest is fully GPU resident at completion. Whole-device sampling records
2856 observations, peak2963MiB, minimum free2835MiB, zero512MiB reserve violations
and zero unavailable telemetry observations. No GPU offload is hidden as co-fit.

The CPU sampler has a600-second bound, so an additional read-only tail sampler
covers Qwen's final phase in `cpu-tail.jsonl`; the original files are unchanged.
Post-cleanup unavailable samples remain recorded. Across135 available samples,
service mean/peak CPU=297.54%/381.84% and Qwen=32.29%/358.88%;100% is one core.
These cover loading/supervision, have a gap between the original and tail
samplers, and are not exact per-model CPU-time or energy measurements.

## Method and limits

All candidates receive the same `context`, full `text` and `focus` JSON. IDs,
gold, split, source-kind fixture and annotation adequacy never enter inference.
The common `sentence-quote-window-v1` splitter retains punctuation and exact
Unicode offsets; it splits sentences/semicolons/newlines outside quotes and
brackets. It does not claim learned minimal spans or atomic semantic units.
Conjunctions remain inside windows, allowing multiple intentions. Mixed
incompatible target/modality values are unresolved and escalate.

The source wrapper is a current PostgreSQL-owned `decision_shadow` job. Every
classification and embedding probe has its own finite spent reservation and
post-call source checks. CPU reservations cover seven 2048-token rows; Qwen
reserves 2048+128 tokens and its complete exact wire bytes. First attempts allow
60 seconds, warm attempts 20 seconds, always bounded by the earlier context.
Research owner-day turns are declared before the first reservation; ordinary
policy limits are unchanged. Recorded ledger totals include embedding probes.

GLiNER uses native independent sigmoid labels with described Russian names,
plus separate softmax modality and target groups. Reviewed d1 uses its native
`noul` boolean head for intent and concise described choices for modality/target.
All seven upstream encoded rows are checked for instruction/option/state
truncation. All seven reviewed upstream code/config hashes remain unchanged.
Only the tensor adapter/typed schema changes; no new dependency or checkpoint.

Qwen uses the same pinned Q4_K_M weights/Ollama/tokenizer as v2, with fixed
non-thinking raw chat serialization. Output is five named booleans and two
integer enums. Duplicate/unknown fields, invalid types and incomplete output
deny observation. Conditional token scores require every alternative at each
field. A valid answer with missing alternatives stays observed and escalated;
no missing probability is fabricated. Scores are uncalibrated. The adapter is
research-only and does not implement a production Qwen transport.

Binary presence remains 0.5. Non-escalation needs minimum score 0.65 and margin
0.15 across all seven decisions; unresolved/non-direct statements escalate.
Rules are a separate frozen literal baseline without statistical confidence.
The provisional screen requires per-intent P>=0.90/R>=0.80, joint correctness
>=0.90, zero critical errors and wrong non-escalated observations, and useful
nonzero coverage. Screen completion is not production acceptance.

## Independence

The original 24-dev/72-design-event corpus and zero-prediction freeze are
retained. Its examples were co-designed with the task and literal baseline.
Model adapters were repaired on dev only. The final file
`russian-interpretation-heldout-v3.json` retains the exact same 24 dev events,
but contains 72 newly written test events after the rules/semantic protocol
were fixed. It has 28 dev and 76 test quote windows. Final corpus, schema,
annotation rules, literal rule source and semantic adapter/metric sources are
SHA-pinned before any final held-out predictions. Neither gold, class IDs,
rules nor thresholds are edited from those predictions.

This is one-author synthetic data, not independently human-reviewed real traces
or a production-distribution claim. The final labels/source hints are fixtures,
not actual owner authentication or completion evidence. It is not comparable
as an accuracy percentage to the old nine-class task. Current-controller cost
and actual saved calls remain unmeasured/zero respectively.

## Retained development failures

- Qwen preparation initially used root with all capabilities dropped against an
  owner-writable public model directory; manifest creation was refused. It now
  uses the directory owner's UID/GID. No inference occurred in that refusal.
- First zero-inference PostgreSQL contract check failed in a late-source test
  because its intentional update omitted `ExpectedRevision`; repaired test
  preserves the actual source fence. No tensor inference occurred.
- First v3 worker emitted replies inside stdout redirection, causing timeouts.
  Interrupted partial checkpoint/log/snapshot are retained; stopping the owned
  runner then removed its disposable PostgreSQL, producing a terminal finish
  error. This is deliberate interruption, not a production outage.
- Repeating full common instructions seven times in GLiNER's combined schema
  exceeded context on some dev windows. A second interrupted dev checkpoint
  retains those explicit InvalidArgument refusals and actual successful rows.
- Repeated generic absent/present choices strongly biased GLiNER toward present.
  Native independent labels and Russian semantic names replace that dev format.
- d1's reviewed encoder has per-option and question limits even with a 2048
  overall context. Long dev instructions/options were rejected, not truncated.
  Concise descriptions and the native boolean head preserve exact input checks.
- Qwen's first GPU runtime attempt timed out at 60 seconds. Positional arrays
  also confused fields on dev. Named typed fields were fixed before final test.
  Later first-profile timings are warm residency, not cold-load measurements.

Historical snapshots and reports remain under
`/srv/storage/butler/agents/decision-interpretation-v3`. No failed result is
rewritten as a successful inference, and no held-out quality replay is used to
tune the schema. Pin/source records describe each actual version.

## Validation and retained sources

Independent count/metric audit confirms304 unique profile/case/window pairs,
zero failed final RPCs, and exact intent/joint totals from original predictions.
Final Linux/PostgreSQL checks pass140 test events across six packages with race
detection and zero failures; `go vet` also passes. Ten explicit opt-in skips
cover actual old/new model runs, token export, workload, saved-output audit,
old FunctionGemma, old extraction quality and external pg_dump/restore. New v3
comparison, tokenizer export and saved-output audit were executed separately.
Historical quality runs are not repeated. Windows targeted decisions/localmodels
checks pass; the existing POSIX private-token permission test fails on Windows
and passes in the actual Linux service check.

The first broadened six-package check crashed its disposable PostgreSQL during
another migration set, returning unexpected EOF and then recovery refusals.
Its original JSONL is retained; PostgreSQL logs had already been removed by the
old cleanup, so the exact crash cause is unknown. The repeat uses2GiB temporary
PostgreSQL storage/memory instead of512MiB/1GiB solely for this expanded test.
It passes and retains container logs before cleanup. No model admission limit,
production service, source corpus or classification is changed by that repair.

Canonical evidence:

- Final dev: `decision-replay-20261014T093920Z-1634184`, source snapshot
  `20261014-163911-575`.
- Once-only final test: `decision-replay-20261014T095031Z-1675485`, same source
  snapshot; all observed responses stay original.
- Final six-package race/vet: `decision-replay-20261014T101610Z-1771146`, snapshot
  `20261014-171602-307`; additions are the zero-inference offline score audit,
  documentation and broader validation runner.

Raw final report, all conditional token alternatives, earlier dev versions,
interrupted checkpoints, immutable freeze/source hashes, GPU/CPU data, separate
embedding records and Linux race outputs are checked in here. The original
runner snapshots retain all preparation versions. The main lab services stay
healthy; owned experiment containers/networks are removed and GPU returns to
17MiB used. Public weights and reports remain available.

## Reproduction

Use a fresh isolated root and existing reviewed weights/dependencies. Qwen
preparation accepts `interpretation`; an existing owned research container can
refresh exact tokenizer preflight with `refresh-interpretation-preflight.sh`.
The runner mode is `interpretation dev co-resident`, then
`interpretation test co-resident`. It removes only its disposable model/PG
containers/network. The separately prepared Qwen container/network is explicitly
removed after completion; public weights/pins/reports are retained.

Placement is Qwen GPU plus accepted EmbeddingGemma2 CPU/FP32. Weights, revision,
preprocessing, dimensions and vector index identity are unchanged. Every Qwen
window is followed by a separately admitted CPU embedding probe. Whole-device
GPU and shared-service/Qwen CPU sampling accompany the reports. Separate
containers do not demonstrate combined cold-load/RAM admission in one ordinary
production lane; sustained mixed workloads and load/switch timings remain future
measurements. Electricity/hardware cost and exact process CPU time are unmeasured.
