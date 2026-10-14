# Source-bound event interpretation: issues 27 and 28

Version: `butler-event-interpretation-v3`. Status: deferred research as of
2026-10-14 by owner decision; production consumer disabled. The selected model
configuration remains EmbeddingGemma2 and Gemini 3.8 Flash High. See
[the decision and resumption conditions](../ai/model-selection-20261014.md).
This contract is retained for future work; historical experiments remain reproducible.

Issue 27 owns candidate representation and validation: exact Unicode quotes,
source/revision/access/hash, interpretation version, model or rule identity,
event/ingestion/extraction times, optional unresolved fields and deduplication.
Issue 28 owns comparison of semantic observations and their measured usefulness
to the controller. PostgreSQL/source ownership and separate candidate acceptance
remain authoritative; neither subsystem grants an action or verifies completion.

## Semantic questions

An event can contain multiple intentions: `request_action`, `change_constraint`,
`stop_work`, `ask_information`, `provide_information`. Presence is classified
independently, rather than selecting one winner. Zero intentions is allowed for
greetings/acknowledgements. Each observation also has modality
`direct|tentative|quoted|negated|conditional|unresolved` and target
`whole_current_task|current_operation|other_task|unresolved|none`.

Trusted origin/actor/source policy is input to the owning code, not a predicted
class. Foreign text remains DATA even when its semantic intent is imperative.
Completion claims are information. Actual verification needs current task,
constraint and evidence versions through the existing result contract. Urgency,
executor selection and event relevance are separate future questions.

## Bounded common experiment

The initial common adapter splits source text at sentence punctuation,
semicolons and newlines outside quotes and brackets. Conjunctions/commas stay
within a window; multiple intentions may coexist. Windows retain full source
text and context, and exact code-derived Unicode offsets. This is deterministic
source segmentation, not learned span extraction or guaranteed atomic meaning.
Mixed incompatible targets/modalities stay unresolved and escalate. Annotation
rules specify the observable window and are evaluated separately for adequacy.

All candidates receive identical context/text/focus, excluding case IDs, gold,
split and annotation notes. GLiNER uses its native independent sigmoid intent
task plus softmax modality and target tasks, with pinned Russian label names
mapped to the fixed IDs. Its combined input contains the complete common policy
once, with local task questions/descriptions; repeating the common policy seven
times is not a usable complete-input encoding. Reviewed d1 uses five native
`noul` boolean questions plus modality/target choices with concise descriptions,
with exact input preflight for every upstream encoded row. Returned scores are
five positive-class probabilities and complete modality/target distributions:
16 fixed IDs fit the existing bounded classification RPC. No proto changes.
Qwen uses five named booleans and two integer enum fields over the same semantic
questions. Missing token alternatives leave confidence unknown. Grammar-valid
output is still fallible. Seven decisions are retained independently; there is
no argmax competition between the five intentions.

Each window is independently admitted under a current source-bound finite
`decision_shadow` job. CPU reservations conservatively cover seven 2048-token
rows, including d1's actual multi-question serialization; Qwen reserves 2048
context plus 128 output tokens. Complete wire input, earlier caller deadline,
post-call source checks and immutable spent reservations remain mandatory.
The research fixture declares its planned finite turn/day allowance before calls.
Embedding probes are explicitly owned measured placement operations, not savings.

The common source envelope validates every quote against its current original
text and offsets. The initial experiment returns shadow candidates only; it
does not implement new durable memory publication or controller shortcuts.
Existing issue-27 source/job/publication/dedup/privacy tests remain required.

## Evaluation and promotion

Report availability separately from quality; per-intent precision/recall, exact
intent sets, modality/target accuracy, joint window and whole-event exactness,
false direct stop/initiative, missed stops/constraints, quotation/proposal errors,
and incorrect non-escalated observations. Source-span integrity is a validator
measurement, not extraction accuracy. Confidence is uncalibrated; binary presence
uses 0.5, and every decision must meet score 0.65/margin 0.15 for non-escalation.
Unresolved targets, conditional/tentative/quoted/negated windows always escalate.
Ordinary current controller quality/cost and actual saved calls remain unmeasured
until a separately authorized baseline is executed. All accepted input is retained.

Use a fresh frozen corpus authored before predictions, with a development split
and a separate held-out split. Record hashes, annotation rules, simple-rule
baseline and fixed thresholds before inference. Compare an unchanged rule
baseline on both splits; development adapter repairs must be documented before
the first held-out inference. Held-out labels/prompts/thresholds are not tuned.
Synthetic independently authored data are not human-reviewed production traces.
The original co-designed 96-event fixture is retained as development evidence.
The final corpus keeps its 24 dev events and replaces the 72 held-out events
after the literal rules and semantic protocol are fixed, before any predictions
on that final split. Model-specific protocol choices use dev only; rule source,
semantic IDs, annotation rules and thresholds remain unchanged on final test.

The provisional research screen retains precision >=0.90, recall >=0.80 per
intent, joint correctness >=0.90, zero critical misses/false direct actions,
zero wrong non-escalated observations, and a nonzero useful coverage. Passing a
finite synthetic screen does not demonstrate rare-error safety or net savings.
Production scheduling and ordinary model catalogs stay disabled pending real
trace review, calibrated selective risk and measured end-to-end benefit.
