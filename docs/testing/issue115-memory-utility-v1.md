# #115: prospective utility measurement, v1

Status: **offline development protocol; no native quality qualification**.
Motivation and immutable baseline: [v6 audit](../../experiments/memory115/2026-10-29/evaluation-validity-audit-v1/README.md).
Implementation: [memory-utility-eval](../../cmd/memory-utility-eval/README.md).
The additive [complete development cohort](issue115-utility-native-2026-10-30.md)
now defines the separate 72-slot controlled/native collector and its bounded
descriptive admission. Historical quality qualification remains separate.
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

## Prospective v2: actual Core/Broker observer

`internal/acceptance/utilitytrace` captures the exact owning input, task/run,
tool calls, action bindings, effect/dispatch/evidence records, pinned memory
sources and current heads, model attempts and final from one repeatable-read
transaction. Binding is frozen before execution. Actual input must equal the
suite's public projection; task/owner/session/revisions and original file digest
must match. Read-only current authority/source checks apply to the closed run;
the observer never manufactures a live session lease. A trusted collector seal
identifies the original snapshot; it is not a signature against an adversary who
also controls the expected seal. Model-supplied observation flags are inadmissible.
An explicitly blocked owning input with an unresolved effect may also be captured:
it retains UNKNOWN with completion false and cannot count as task success.

Each result is corroborated against Broker's normalized-result digest, execution
action and completed dispatch. Search candidates and full pinned opens remain
distinct. The pre-run one-to-one mapping translates only actual Catalog citation
identities into oracle identities; raw final bytes remain sealed and no answer,
effect or check is repaired. Missing/foreign/changed evidence fails closed.

V2 generates a prepared-artifact oracle marker `@current-verified-receipt`:
the actual final ID must match the actual current prepare/check chain. This is
needed because a freshly allocated artifact ID cannot be known before execution.
V1's fixed-ID semantics and all frozen results remain unchanged.

`scripts/dev/memory-utility-trace-lab.sh FRESH_ROOT` builds actual Broker and
runtime test binaries, then runs `TestMemoryUtilityActualCoreBrokerTraceIntegration`
with race detection in a fresh private PostgreSQL on an internal Docker network.
For an exhausted Docker default pool, set `UTILITY_TRACE_SUBNET` to an independently
checked unused subnet. No host ports, provider credentials or native requests are
used. Seven controlled cases cover memory read, inspect and prepare/check with
correct/incorrect answers, plus generic rejection retaining UNKNOWN. Corruption
controls remove receipts, substitute results/arguments, change the owning run or
recipient files, remove pins/heads and introduce an ambiguous source mapping.
Raw private snapshots, oracle, observation, scores and failed attempts remain in
the fresh root. Synthetic transport usage must not enter the native spend ledger.

This bridge supports memory.search/open and the reviewed synthetic project
recipient. At that bridge revision, skills.load, arbitrary HTTP tools, lifecycle mutation scenarios and a
typed verified-negative recipient need additional evidence mapping. Unsupported
tools fail closed. This seven-case integration is not a complete 24-case corpus,
a native A/B/C comparison, semantic retrieval quality proof or issue closure.
Native admission still requires a frozen denominator/order, full cost journal
correlation, all arm mappings and independent review as specified above.

### Exact skill loading bridge

The subsequent `skills.load` bridge corroborates Core's ref-only durable result
with the exact pre-bound reviewed manifest, current Catalog body/state, owning
`skill_usages`, action binding, completed dispatch and confirmed Broker evidence.
It reconstructs the original result only to compare its normalized digest; this
does not rewrite the raw observation or final. Current source/lifecycle fences
remain required. Loading alone gives no historical citation or task success.
An actual stage/test/activate/load/approval-recovery fixture and eleven corruption
controls exercise this bridge without native inference.

Full prospective native A/B/C still needs all 72 scheduled observations (24×3),
source preparation/current-document access for all families, lifecycle materialization,
skill search mapping, typed negative/unknown reconciliation fixtures and full ledger
admission. The existing historical 48-control collector and prose outputs cannot
stand in for that cohort. No native entry should be admitted while any of these
paths lacks evidence mapping; retain previous raw results and expenses verbatim.

### Search, currentness and typed rejection observer gates

The next additive bridge supports real Broker `skills.search` followed by load.
Every exposed description is reconstructed from a frozen digest/current manifest
head, preserves stable ID order and matches the Broker's normalized original
index result. A source revision update and subsequent Catalog revoke must each
invalidate a previous successful memory observation. This is a real currentness
test, not just mutation of observer JSON.

The test recipient's exact GET `validate_ready=maybe` supplies a typed validation
format, resource, requested value, `unsupported_ready_value` and an absence flag
derived from its actual empty effect store. The observer requires all of these
plus the current Broker read evidence; a wrong final remains unsuccessful.
The existing untyped mutation HTTP400 still creates UNKNOWN, not verified absence.
These observer gates do not implement the complete native 72-slot collector,
current-document serving for all families or an original-unknown reconciliation
lineage. Native A/B/C remains a separate uncompleted gate until those are wired.
