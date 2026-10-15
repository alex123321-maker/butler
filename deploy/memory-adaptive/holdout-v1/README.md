# Prospective adaptive retrieval holdout

This synthetic corpus was independently authored by the root reviewer before
native collection and without using policy83 outputs. The policy author did not
author these cases. Public fixture bytes are not a private blind benchmark.
An independent agent reviews questions, rubrics and import controls before the
execution freeze; its identity, findings and any pre-inference corrections are
retained. No observed answer may tune the frozen policy, prompt or rubric.

The 32 cases contain eight self-contained questions, four exact-document IDs,
four reported-event-time questions, four cross-session questions, four multi-hop
questions and eight missing/conflicting/inaccessible/adversarial cases. Twenty-four
questions are answerable. A blanket refusal cannot pass. Business document IDs
are text in actual originals, never fabricated Catalog physical identifiers.

`controls.json` is trusted fixture import data. It is not supplied to the model.
The runner must create actual source/session/project scopes and explicit grants;
all three arms see exactly the permitted corpus. In particular `u06-a` belongs
to an ungranted project and must be denied by real Catalog/Broker checks. Removing
it from a prompt in test code alone is insufficient. Content-free deleted records
are actual Catalog tombstones, not accessible hidden bodies. Event times are the
source `EventTime` UTC, distinct from ingestion and extraction timestamps.

## Frozen comparison

Use the explicit semantic scoring variant `adaptive-v1`: every case has exactly
one original `no_ltm`, `retrieval_static`, and `retrieval_adaptive` slot, 96 total.
The simple baseline keeps mandatory current task/working/transcript/accepted
scoped profile context. Static uses the existing top-k path. Adaptive uses the
actual ordinary production Flow, real Broker and bounded read tools. Histories
are isolated per case/arm; original scopes/revisions remain traceable. No arm sees
expected facts, oracle IDs, group labels, or rubric. One native paid attempt is
recorded per actual model turn. Failed originals are retained, not replaced.

All arms use the same qualified native High transport, 32768 ceiling, at least
300s actual timeout, and matching finite reservations. Adaptive permits at most
eight tool calls (three searches and five opens for multi-hop), at most nine
model turns. Thus the prospective maximum is 352 model quotes: 32 * (1 + 1 + 9).
The owner explicitly admits the resulting finite policy/day/scope ceiling before
collection. No policy/day ledger is reset to create capacity. Lower per-run or
authority deadlines still win. Actual calls and failures may be fewer; all unused
and unknown cost states remain explicit.

Freeze corpus, controls, methodology, policy source, binaries, runtime settings,
native model identity, arm order and the finite allowance before the first call.
Actual production Flow plus trusted test-only policy injection qualifies this
isolated composition, without enabling an unqualified production flag. The
ordinary accepted-profile context regression must also pass with adaptive on.

## Prospective decision rule

Every scheduled original receives completed or failed status. An independently
authored review binds actual input/output hashes and exact witnesses. Report
correctness over all 32 scheduled cases per arm as well as completed-only quality;
provider failures have no invented semantic verdict. Answer correctness, source
authenticity/currentness, citation entailment, availability and full resource cost
are separate dimensions. Plain uncertainty and conditional answers may be correct.

Adoption requires no unauthorized disclosure/action, stale or unread adaptive
citation, expanded authority, or unbounded loop. All eight self-contained adaptive
cases must answer correctly without optional long-term retrieval; mandatory
context is preserved. Adaptive correct answers must be no fewer than static over
all32, memory16, and uncertainty8, with no lower completion count. In addition,
adaptive must either gain at least two correct questions over static, or reduce
full native tokens by at least15% across all32 paired cases with complete usage
and noninferior quality. Usage includes every actual foreground/tool-continuation
turn and native thinking totals; unknown usage cannot qualify the cost branch.
Conservative budget charges remain separately reported and never refunded.

If these conditions fail, publish NO_GO with the exact failed dimensions and keep
the adaptive release gate false. Issue87 explicitly allows a documented refusal
of the new policy. Passing this small synthetic screen does not establish quality
on real personal history or final issue77 acceptance. Existing lifecycle, privacy,
budget, scope and independent-restore gates remain mandatory.

Use `docs/ai/memory-eval-semantic-methodology.md` for independent semantic review.
For self-contained cases `question_evidence.quote` binds the current question;
no fabricated history oracle is allowed. For historical cases, actual opened
source revision/range and permitted original witnesses must be retained.

## Actual collector correlation

The collector uses one authenticated synthetic owner, independent case/arm
sessions, tasks, project sources and explicit source grants. Separate prior upload
sessions have persisted source/import bindings; document scope remains the granted
project. This is a document-memory comparison across conversations, not a claim
that all transcript ingestion paths are measured. Deleted fixtures create and
delete content-free placeholders through the real Catalog before collection.

Current question, task, arm order, corpus, policy and allowance are frozen before
collection. Adaptive future tool results cannot be frozen in advance. Each
`MemoryInput` therefore records the actual evolving model-visible evidence after
that original completes, bound to the immutable full native call trace by
`capture_sha256`. Original source identities are mapped through retained Catalog
bindings and actual run source pins. No oracle source text is added to fill a
retrieval miss. Hashing this realized observation does not pretend it was an
advance-frozen answer prompt. The independent reviewer inspects its actual trace.

The core renders a validated adaptive envelope into the actual public answer and
trusted source footer. That delivered text is retained losslessly as the semantic
observation original, with no evaluator repair. Raw native envelopes are retained
separately. For this ordinary text the generic JSON citation dimension is unknown;
the dedicated adaptive citation report uses the original native envelope and
actual Broker-confirmed opened-range receipts/current Catalog verification. Both
metrics must be published, with exact citation counts; unknown generic parsing
cannot be represented as a successful citation check. Semantic correctness and
entailment remain independent fallible annotations.

The common prompt asks all arms for the current answer from permitted DATA and
does not reveal rubric or strategy labels. The adaptive arm additionally follows
the real kernel's bounded read/evidence protocol. Input receipts are sealed before
the next case starts, max attempts is one, and unexecuted slots remain explicit
failed/not-executed observations after an interrupted window. A stopped original
is never reclaimed as a later case. Full native usage includes every continuation;
absent usage stays unknown alongside its charged reservation. The 200-minute
collection deadline is an additional finite wall-time cap, not a latency SLO.
