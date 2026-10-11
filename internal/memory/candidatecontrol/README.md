# Private candidate publication control

`memory.propose_candidate` revision1 is a fixed local owning control, target
`broker://memory-candidates-v1`. Inputs are source ID/revision, fact key, a bounded
JSON value string and summary. They are DATA. Model fields cannot supply owner,
scope, acceptance, confidence, deduplication identity or authority. Output is only
an opaque claim ID and `candidate` status.

The actual Broker prepares/authorizes the exact contract, claims its permanent
effect and reserves shared budgets before calling the fixed authenticated
`/internal/memory-candidate` endpoint. Orchestrator reconstructs current Session
ownership and the original private accepted-input claim from durable state. It
checks the exact tool-dispatch slot, task/assignment/constraints and source graph
in the publication transaction. Only a source already pinned by that run can be
used; scope comes from the source. A privacy pin does not imply model body access
to every ancestor. Current `memory_extract` access remains independently required.

Catalog uses the actual effect ID as trusted deduplication key, stores confidence0
and never updates the accepted memory head. Owner acceptance uses the existing
protected memory API and exact current head CAS. Source deletion clears candidate
text using normal catalog privacy rules. Recognized credential keys/patterns deny
the proposal without rewriting it; secret-origin sources remain denied.

Private payloads reject extra/duplicate/case-alias/null/trailing fields. The
operator endpoint is exact, HTTP on literal loopback or HTTPS; no ambient proxy
or redirects carry service authentication. The control is `prepare`, unsafe retry
and `local_runtime_ack`. A lost response leaves Broker effect reconciliation
unknown; no automatic replay or acceptance is implied. Exact private retries
retain the original neutral candidate and cannot change a body or owner decision.

Separate operator-only `BUTLER_MEMORY_CANDIDATE_ENABLED` defaults false in both
services. Broker additionally needs `BUTLER_MEMORY_CANDIDATE_CONTROL_URL`, private
action authority token/context and PostgreSQL. Orchestrator requires the private
token, current catalog/deletion anchor and publication schema. Database settings
cannot enable it. Delegated context-only contracts remain restricted by their
exact tool/purpose grants. No model call or legacy extraction worker is started.
