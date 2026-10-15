# Bounded adaptive memory reads

Issue87 is a prospective, default-off consumer of the existing memory read
Broker. `adaptiveRetrievalQualified=false` is owned by code; neither operator
flags nor stored settings can bypass qualification. Main activation is separate
issue92 work. The independent holdout and adoption/NO_GO rule live in
`deploy/memory-adaptive/holdout-v1/README.md`. No result is claimed by this design.

The Orchestrator chooses a transparent current-message intent plan. Independent
questions omit optional episode/document search and memory tools. Current typed
task/goal/constraints, transcript, working memory, summary and scoped accepted
preferences keep their existing loading and authority checks. Necessary profile
reads are distinct from optional retrieval; a no-search run is not zero DB work.

Literal identifiers use bounded catalog search. Reported event dates may impose
UTC `[event_from,event_until)` on the actual source `EventTime`, excluding null
dates. Ambiguous years require clarification. This never proves valid-time or
known-time; issue88 owns those separate typed semantics. General recall uses the
existing guarded semantic/hybrid reader, including explicit degradation signals.
Only comparison/join intent permits further query decomposition. Model queries
may refine discovery within this plan; they never carry owner/scope/grants.

Each immutable plan binds owner/session/task/constraint revisions and a digest
of sanitized current input. Migration074 retains only bounded plan metadata,
call/response counters, operation hashes and opened range/source receipts. Recall
permits four calls/one search; multi-hop permits eight calls/three searches.
The initial turn plus eight continuations is at most nine model turns. Response
JSON totals at most65536 bytes; optional reads have a120s window from first
reservation. Existing owner/day/tree/native budgets and earlier deadlines still
apply. Reservations survive restart and failed reads are not refunded. This
module does not change provider quotes, Google usage or billing semantics.

The owning read service proves the exact permanent Broker dispatch, durable
input claim and current worker binding before reserving. Semantic IPC happens
outside its SQL read transaction and uses the existing local admission/target.
Candidates contain only server-derived chunk IDs/revisions. The owning reader
reauthorizes current source scopes, exact grants, complete parent graph and
privacy/deletion state before publishing. The dispatch and all pinned sources
are checked again after waits. No textual permission, source reference or
search expansion changes authority.

Search is discovery. Only successful actual `memory.open`/`memory.neighbors`
results receive `read_ref`. A retrieval answer supplies a small public JSON
answer/citation/unknown envelope, never hidden reasoning. Each citation identifies
a short assertion, opened receipt, exact quote and Unicode offset. The final
verifier requires actual Broker effect confirmation and exact result digest,
reopens that same current source revision/range, and compares bytes and offsets.
Conflicting/incomplete evidence and unproved historical periods need an explicit
public limitation. These checks prove source identity/currentness; semantic
entailment, answer correctness and calibrated uncertainty are separate eval
dimensions. Partial answer deltas wait for verification. All continuation paths
converge on the same verifier before finalizing; lost publication ACK recovery
uses the already validated finalizing checkpoint and ordinary atomic source/task
guards. A failed proof cannot publish an unchecked answer.

`BUTLER_MEMORY_ADAPTIVE_RETRIEVAL_ENABLED` is operator-only/defaultfalse and also
requires memory reads. Isolated tests explicitly compose the production Store,
ordinary Flow and actual Broker while qualification remains false. The native
freeze must bind source/binary/corpus/control hashes and finite prospective
capacity for every original arm; failed originals and unknown spend are retained.
