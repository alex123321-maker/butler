# Semantic evaluation of Butler memory

Version: `butler-memory-semantic-v1`. This protocol evaluates answer correctness
against dated original evidence, not resemblance to a reference sentence. It is
an observation protocol; it neither trains a model nor authorizes external effects.

## Evidence and questions

Each prospective case fixes its question, as-of time, current task, chronological
public source history, required facts, forbidden inferences and sufficient oracle
sources before any answer is observed. Another reviewer checks the rubric and
ambiguities before execution. The runner never sends expected facts, source
selection rules, split names or review labels to the answering model. Oracle mode
intentionally supplies only the declared sufficient original sources.

Self-contained current questions may explicitly declare `evidence_basis=question`.
They are answerable without an oracle history source; the corpus must not invent
one to satisfy a history-only loader. The independent grounded review supplies
`question_evidence.quote` copied exactly from the actual current input. Original
case/input binding and quote authenticity remain checked, while whether an answer
follows from that question remains a fallible semantic annotation. Older cases
default to history evidence and retain their original oracle requirement and bytes.

An ordinary language explanation of missing information is acceptable. A missing
fact is not a negated fact. An uncertain or partial answer must not silently become
a positive claim. Plausible but unsupported assertions fail grounding even if they
happen to match the full corpus. Superseded facts are evaluated at the question's
time; source authenticity does not itself establish relevance or entailment.

## Independent review dimensions

For every completed original, the reviewer separately records:

- `verdict`: correct, partial, incorrect or unresolved for answering the question;
- `context_verdict`: correctness given the evidence actually visible to the model;
- `corpus_verdict`: correctness relative to the permitted complete history;
- `grounded`, `complete`, `uncertainty_correct`, `action_correct`: nullable booleans;
- `evidence_available`: whether sufficient evidence reached the actual input, not
  merely whether a relevant source ID appears in a retrieval receipt;
- `globally_answerable`: whether permitted full history and current task suffice;
- `retrieval_miss`: sufficient evidence exists but retrieval/bundling omitted it;
- `format_canonical_mismatch`: an old lexical/abstention/citation-count rejection
  which is not by itself a semantic error;
- `failure_origin`: none, retrieval, answer or rubric_ambiguous (a baseline with no
  memory is reported as such, not called an embedding failure);
- rationale and exact original source/revision/quote witnesses, with provided
  versus corpus-only availability. Source quotes and any supplied model citations
  are checked independently. A current-task witness is recorded separately.

A safe refusal with an answerable history is context-correct but incomplete and
cannot count as a fully successful question. A justified explanation when the
history genuinely lacks the requested fact can be correct and complete. Code or
format failures do not imply fabricated facts, unsafe actions or violated tasks.
Action-code proposals remain distinct from executed tool effects.

## Reviewer validation and limits

Deterministic code checks bind every review to corpus, methodology, exact input
and unmodified output hashes; require exhaustive unique completed observations;
and check original quote authenticity. This does not mechanically prove semantic
entailment. The independent reviewer must justify judgments using sources, and
mark genuine ambiguity unresolved. A second model/agent is fallible, not a human
annotation or a source of authority. No model-to-model vote is ground truth.

Before prospective execution the reviewer evaluates a calibration set containing
correct paraphrases, altered dates/entities/negation, obsolete information,
authentic-but-irrelevant quotes, supported uncertainty and an incomplete refusal.
All disagreements and a fixed sample of accepted answers receive independent
review. Record calibration misses and adjudication; do not silently repair them.

Retrospective review is diagnostic on already exposed cases. Preserve original
strict scores and report reclassification separately. A changed method cannot
turn these cases into an untouched holdout. Fresh cases are frozen before native
calls, are not used to tune prompts/retrieval, and provide only a small synthetic
acceptance sample. Public repository controls are not a private blind benchmark.

## Fresh execution and attribution

Use the same model, decoding, transport, five-minute per-turn timeout and answer
instructions in no-memory, ordinary retrieval, and sufficient-evidence oracle
arms. Answers use a small JSON envelope with free `answer` text; citations are
optional and their absence alone is not a factual error. No canonical abstain,
confidence or action enum is required. No external tool definitions are supplied.

Histories enter the real Catalog, parser, owned indexing job, local embedding
backend, PostgreSQL/pgvector HybridReader and current bundle builder. Use a fresh
isolated database, preserve source/session identities and trace the persisted,
indexed, selected and visible evidence. A separate read process reconnects after
writer exit to test persistence rather than reusing process memory. New sessions
and owner isolation are explicit; correction/deletion probes retain lifecycle
receipts. This is a document-memory contour, not proof of every transcript,
profile-extraction or consolidation path in the product.

Fix the case/arm schedule and repeated probes before execution. Retain every
failure; never retry to replace a bad answer. A provider availability failure may
be followed by a different predeclared case, but the failed entry remains failed.
Safety/authentication/retention failures stop execution. No expired original run
is reopened. Independent semantic review follows collection of immutable originals.

Report question success over all scheduled eligible entries AND semantic quality
among completed answers, answer completeness, unsupported assertions, retrieval
evidence recall, unresolved judgments and repeated-probe consistency. Timeout and
provider failure count against end-to-end availability; they have no invented
semantic verdict. Do not discard unavailable entries from the practical denominator.

Keep latency, availability, actual effects and resource measurements separate from
factual quality. Owner-declared incremental model billing is zero for local
embeddings and the subscribed Gemini model. This pricing premise is not a measured
invoice or free host hardware; tokens/calls remain operational measurements and
do not themselves make an otherwise correct answer fail. No arbitrary historical
call-count or 60-second p95 gate is inherited. Five minutes is an execution timeout,
not a desired response-time target. This version reports a diagnostic profile,
not an automatic universal GO/NO_GO or a claim of long-lived-task completion.
