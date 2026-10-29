# #115: prospective utility measurement, v1

Status: **offline development protocol; no native quality qualification**.
Motivation and immutable baseline: [v6 audit](../../experiments/memory115/2026-10-29/evaluation-validity-audit-v1/README.md).
Implementation: [memory-utility-eval](../../cmd/memory-utility-eval/README.md).
This changes the research measurement contract only. Product memory, Broker,
run lifecycle, provider limits and old results are unchanged.

## Frozen questions and evidence

Use 24 cases: four families (historical recall, preference with an exception,
resumption after a checkpoint, reusable multi-document procedure), each with five
conditions (useful, superseded, deleted, distractor in another scope, current input
sufficient), plus four integration regression oracles. The generator commits random
answers in a private suite and emits a distinct model-only projection. Fresh random
values prevent pretrained factual recall; they do not create a statistically
independent holdout from public templates.

Before native work, the preparation collector must persist the past session/source,
produce any notes and skill, freeze them, then use a new reader session. Oracle,
family/condition labels and expected answer never enter model inputs, tool catalogs
or descriptions. Current source documents are available identically to all arms.
Do not hide legitimate current ABI data to make memory win. In the procedure family,
three current documents let A derive the same composite answer; a verified recipe
can save discovery in B/C. Stale/deleted notes cannot prevent current derivation.

A receives no prior experience, B receives permitted frozen notes, C receives the
same notes plus an eligible reviewed skill. Use the same current task, recipient,
model/configuration, limits, time, tool behavior and reset state per paired case.
Maintain the distinction between this deployment-package comparison and a causal
ablation: D is an additional diagnostic with B's catalog/guidance and empty memory.
A separate supplied-correct-evidence diagnostic may identify search versus use
failures; neither diagnostic is reported as ordinary A/B/C performance.

Balance and seed-randomize arm order before inference; reset caches/session and
all task state. Declare repeats and all slots before calls. Do not count repeats
as independent families. Every scheduled missing/failed case remains in the main
denominator. Provider-conditioned slices are labeled exploratory and cannot replace
the main results. Freeze model instructions, response schema and all evaluator
hashes before native calls. The present CLI does not perform this admission step.

## Outcome contract

The formal track requires a strict typed JSON final, identical schema for all arms.
The observer independently supplies source/read receipts, current effect/check,
unchanged original and authority observations. The model cannot certify those gates.
Correct value without permitted read/citation is insufficient for historical claims.
An honest A refusal when past knowledge is unavailable is recorded as abstention,
not success at answering an answerable question. For a truly unavailable current
fact, UNKNOWN is the expected outcome. Current-sufficient cases require no read.

Inspect-only checks must report failed when the actual check failed. Lost-effect
checks must preserve UNKNOWN; merely saying the word UNKNOWN is insufficient.
Verified-negative checks need an independently recorded exact typed reason AND
absence receipt; generic HTTP400, UNKNOWN or absence alone do not prove the cause.
The prior recipient does not yet provide this combined contract for control03:
the next integration must add a separate synthetic typed rejection fixture, without
weakening the existing generic Broker UNKNOWN behavior or relabeling old attempts.
Prepared artifacts require the same current artifact/check and unchanged original.

The formal track closes the three demonstrated substring/regex false positives.
It deliberately does not claim arbitrary prose understanding. A later natural-language
track requires separately reviewed rubric/annotations, contradiction calibration,
exact task/effect linkage and disagreement handling. Do not auto-extract a convenient
typed answer from an otherwise contradictory free-text final.

## Find the failing stage before changing memory

The native collector must report separately:

1. Was the needed observation actually persisted with current provenance/status?
2. Was it eligible and indexed after restart under the current resource/grant?
3. Did discovery return it, at which rank and cost?
4. Did the model open the exact version or load an applicable skill through Broker?
5. Did the final use it correctly and did any action pass fresh verification?

The offline evaluator currently checks candidate/opened/application from externally
captured rows. It does not prove stages 1/2 or actual Broker skill loading. Source
references, task/authority versions and actual receipt bytes must be cross-checked
against originals in the future collector, including a check that *all* exposed
source bytes appear in the audit. No observation omission may create a pass.

## Memory improvement hypotheses, not production changes

Select memory for a specific information gap. Prefer a compact useful fact or recipe
with conditions, outcome type, provenance and current source pin; expand original
evidence when needed. Distinguish verified negative observations from UNKNOWN.
Stop optional retrieval once another search adds no usable evidence; report the
remaining uncertainty or use current tools when authorized. Do not weaken source,
authority or privacy fences. Existing read limits remain unchanged in this task.

Measure marginal benefit per retrieved source and per loaded skill. Do not optimize
for mandatory read count. Include no-benefit and distracting evidence controls so
the scoring rewards *skipping* useless retrieval rather than reading everything.

## Quality and full cost

Retain task success, abstention, false assertions, repeated failed actions, denied
source use, candidate/open/read counts, native calls, token totals and latency.
Preserve unknown usage, failed requests and incomplete remaining work. Record
original execution S, note creation N, skill creation K and control Q separately;
compare S+Q(A), S+N+Q(B), S+N+K+Q(C) without summing shared allocations as unique
spend. Report both cold cost and reuse at an observed reuse count, never assumed
amortization. Invoice cost is separate from measured token/work costs.

No historical score, frozen protocol, raw evidence or expense is changed by this
new version. Unit witnesses and model-free tests are checks of the measuring tool,
not evidence that memory improves the agent or that #115 is ready to close.
