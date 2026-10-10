# Structured memory candidates

This optional Go caller uses `butler-memory-v1` on the pinned internal
`local-models:9090` service. It publishes only neutral candidates and cannot
accept goals, grant resources, write profile facts or create mandates. The
measured Russian quality gate fails, so production scheduling remains off.

`Run` requires an actual current durable `extract` job, purpose `memory_extract`,
an exact source revision/access revision/full hash, a private backend and an
independently pinned local target. Every actual IPC window has a fresh shared
owner/task/mandate/job reservation and retains the returned bounded context.
Catalog access preserves the full ancestor manifest and denies changed,
deleted, secret or unauthorized data before IPC and publication. A finite
owner-created job can have no task; a completed task lease is never revived.
Production composition must use `ClaimKind("extract")`, not the index worker.

Fields contain exact half-open Unicode-character offsets and literal text.
Records retain pinned versions, source author/locator, inherited data origin and
separate event/ingestion/extraction times. Missing fields remain unresolved.
Relative dates use only known source event time and an operator IANA timezone;
code identifiers preserve spelling/case.

Quote-level literal negation/conditional/historical/exception markers retain
their own offsets and zero score as rule evidence. They are separate from model
fields and quality metrics. Their scope is `quoted_fragment`: a cue does not
assign polarity to every action, and “можно” never grants permission. Long
paragraphs use bounded excerpts with `quote_clipped`; absent cues never imply
affirmative permission. A full source restriction remains a candidate even if
GLiNER omits its prohibition label.

When model entities share a restriction quote but the model omits their grouped
record, `constraint_fragment` retains those exact annotations and rule evidence.
It explicitly leaves `action_polarity`/`exception_scope` unresolved, rather than
claiming a semantic relation. Model spans use `evidence_method=model_span` and
lexical evidence `literal_rule`; neither source becomes owner authority.

Input has at most 32 UTF-8 windows of 1200 bytes with 80-character overlap. The
worker also counts actual text+schema encoding and refuses over 512 tokens
without truncation. Correlation, exact model/device, scores, labels and source
spans are validated for the complete batch before any publication. Invalid or
partial work publishes nothing; attempted inference stays charged. Zero
candidates is a valid completed publication.

Migration 043 retains content-free checkpoints and exact retry bindings. One
transaction publishes all candidates and completes extraction. Identity pins
all versions and timezone; exact retries return original claim IDs/decisions
without inference. Changed bodies under the same trusted key are rejected.
Catalog deletion redacts source/claim text; this metadata stores only opaque
IDs/hashes/times. Downgrade refuses populated history.

See [the contract](../../../docs/architecture/structured-memory-extraction.md).
Tests use an isolated `BUTLER_TEST_MEMORY_POSTGRES_URL` `_test` database, private
schemas and pool size two. Actual service checks also require the private
single-file token mount. The replay script starts no second tensor worker and
does not touch the main database.
