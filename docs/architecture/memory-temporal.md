# Memory validity, observation and freshness (#88)

This document fixes the implementation contract for issue88. The temporal runtime
is under implementation and remains default-off/unqualified until its acceptance
gates pass. PostgreSQL, existing catalog sources/claims and owner fact heads remain
authoritative. A freshness policy never accepts, rejects or supersedes a claim.

## Time fields

| Field | Meaning |
|---|---|
| Source `EventTime` | Reported source event instant; independent of fact validity |
| Source revision `extracted_at` | Core-recorded ingestion of that immutable revision |
| Claim `created_at` / `ExtractedAt` | Core-recorded candidate extraction |
| Claim `accepted_at` | Authenticated owner decision recording time |
| Temporal revision `recorded_at` | Core-recorded owner metadata or verification update |
| Temporal `valid_from`, `valid_to` | Explicit claimed validity `[from,to)` |
| Temporal `verified_at` | Explicit owner verification or proven Broker read observation |

Existing profile `effective_from/to` describe projection/owner decision lifecycle;
they do not become valid time. Unknown validity has null bounds. An explicitly
declared interval can have an open bound; its null bound does not mean an inferred
date. Raw relative dates, dates without zones and source `valid_time_quote` stay
raw/unknown. Typed controls accept complete RFC3339 instants with an explicit zone.
Later ingestion cannot make a document more true or replace an owner preference.

## Typed read and pin contract

The catalog owns these transaction-scoped methods and the exported types in
`catalog/temporal_types.go`:

```go
ReadAcceptedFactsTx(ctx context.Context, tx pgx.Tx, claimIDs []string, q TemporalQuery) (AcceptedFactPage, error)
CheckAcceptedFactPinsTx(ctx context.Context, tx pgx.Tx, pins []AcceptedFactPin, q TemporalQuery) error
BindAcceptedFactPinsTx(ctx context.Context, tx pgx.Tx, derivedSourceID string, pins []AcceptedFactPin, q TemporalQuery) error
BindNewAcceptedFactPinsTx(ctx context.Context, tx pgx.Tx, derived Source, exactParents []SourceRef, pins []AcceptedFactPin, q TemporalQuery) error
ReadAcceptedFactsForSourcesTx(ctx context.Context, tx pgx.Tx, sources []SourceRef, q TemporalQuery) (AcceptedFactPage, error)
```

IDs are a finite caller-selected set, never an owner/scope grant. Every read
rechecks restore state, the owning run/job/owner proof, all original source
ancestors and current scope. The ordinary source EventTime filter belongs to87;
it does not silently become `ValidAt`. Nil/nil selects the current owner head.
`ValidAt` selects explicit accepted validity at that instant using current
knowledge. `KnownAt` limits source/extraction/decision/metadata knowledge to what
was actually recorded by that instant. Both can be supplied together. Unknown
validity cannot satisfy a dated validity filter and is counted explicitly.

The Broker `memory.facts` contract accepts exact references already supplied in
the current authorized context or discovered by `memory.search`. Discovery is
not a prerequisite that mints access; current Catalog scope and original-source
proof remain mandatory for either route. This explicit lookup has no dependency
on87 natural-language classification or its final opened-citation envelope.

Historical accepted claims can be superseded today, but current privacy,
correction and scope proofs still apply. Logical freshness expiration preserves
that history; privacy deletion denies it. No old source revision or restored
database bypasses the newest independent deletion anchor.

An `AcceptedFactPin` names the exact source revision/hash/access, current owner
head revision/active ID, current temporal metadata revision and the selected
metadata hash plus time-sensitive freshness. A historical metadata revision can
be older than the current head pinned for invalidation. Derived snapshots bind
these pins and the exact query atomically. The075 relational metadata predicate
must cover publication, full pre-HTTP proof, periodic observation and later
reads; retaining only a source-content pin is insufficient. Revisions/time
changes cannot silently refresh an old generated view.

Fresh derivative binding requires a constructor-only, nonserialized receipt for
the exact current top transaction (including nested savepoints), head1/access1,
an active or quarantined immutable candidate, and its complete declared parents.
This does not add the new child to a job's read grant. Pin batches use a savepoint
so caller recovery cannot commit partial protection. Ordinary owned runs bind
accepted fact observations immutably in `memory_temporal_run_pins`; subsequent
reads cannot refresh a prepared run. SQL publication fences and owning full
release checks retain current decision/metadata/freshness proof after waits.

## Freshness and authority

Owner controls classify `stable_preference`, `work_status`, `technical_fact`,
`completed_episode` or `unknown`. Mutable work/status facts require short
revalidation; mutable technical facts use a longer independent policy. Stable
owner preferences do not expire merely from age, and completed episodes remain
historical evidence. Unknown verification stays unknown. Explicit validity of
a one-off task/session exception can end without changing the separate broader
owner preference. An expiry changes current retrieval suitability, not history,
confirmation, task authority or privacy-deletion state.

The metadata update is authenticated owner CAS on claim/head/temporal revision.
The model cannot supply accepted status, revisions, recording time or a freshness
receipt. External revalidation uses an already authorized, budgeted read through
the Tool Broker with a durable observation/result provenance pin. A failed or
unavailable read leaves the fact visibly unverified. It cannot write a new truth,
change a mandate, replay an uncertain effect or remove old spending.

`POST /api/v4/memory/revalidate-read` records an owner's explicit same-fact review
of one existing Broker HTTP read. It requires the real immutable read_result
receipt, exact owner/run/authorization/dispatch binding, the accepted fact pinned
before dispatch, a finite already charged shared tool budget, matching normalized
core-saved result, HTTP2xx and the expected resource hash. An unavailable or
changed response cannot refresh verification. The stored verified instant is the
original Broker observed time, never the later review time. The endpoint performs
no network access, changes no accepted value/head and grants no task authority.
The model cannot invoke this owner control. New external values remain candidates
requiring the existing owner decision lifecycle.

## Current API and operational gate

Authenticated owner controls use `POST /api/v4/memory/temporal` with the typed
`TemporalUpdate` and a delivery identity. `GET /api/v4/memory/facts` accepts a
bounded repeated `claim_id`, optional `valid_at`, and optional `known_at`, each
complete RFC3339 instant with a zone. Ambiguous or relative dates are rejected,
not assigned the server's timezone. Metadata and owner decision history are
included in the private NDJSON export. No model-facing temporal write API exists.

`BUTLER_MEMORY_TEMPORAL_ENABLED=false` is the operator default. A separate
code-owned qualification remains false until composed temporal read,
revalidation, lifecycle and independent restore acceptance is measured. Ordinary
profile/episode adapters can attach explicit temporal/freshness DATA and exclude
only explicitly expired/not-yet-valid current entries; stale/unknown verification
remains labeled evidence. Already bound privacy/decision guards remain mandatory
even when the feature flag is off. No main deployment is activated by these tests.

Agent dated lookup uses the separate canonical `memory.facts` read contract1;
the three existing87 contracts remain byte-identical. It accepts at most8 exact
source references, optional explicit `valid_at`/`known_at`, and returns at most10
bounded accepted fact snapshots with temporal DATA/provenance. It does not return
source bodies from records excluded by a knowledge cutoff. Current privacy,
scope and correction proof remains mandatory. Snapshots receive no `read_ref` and
cannot pretend to be verbatim opened source quotations. Source verification uses
the existing actual `memory.open` protocol independently.

Contract revision2 clarifies interpretation without changing its bounded input
schema or the three87 read contracts. Item `freshness` describes source revision
currency; accepted verification is the separately computed
`temporal[].freshness`, with its explicit `verification_basis`. An owner statement
is not independent external evidence. Revalidation being due does not prove a
stored value false or outside its validity interval. Present `claim_status` is
an owner decision state, not a historical conflict resolver: overlapping accepted
values with `conflict=true` remain unresolved. The native v4 answers and their
original revision1 declaration stay immutable; this clarification needs a new
prospective semantic screen before qualification.

This tool uses the actual Broker/action/input-claim path and common durable tool
budget. When adaptive retrieval is composed it also consumes the same finite
plan call/time/byte counters; it cannot bypass that loop bound and cannot mint
opened-citation receipts. Distinct temporal queries can pin the same claim in one
run, using a UTC-normalized query key, while the64-observation total cap and each
immutable query's stale check survive restart. A changed head/metadata/freshness
invalidates the whole prepared run. Operator endpoint selection is private and
credential-free; both services require temporal readiness independently of read
readiness and the code-owned qualification remains false.

The model-directed temporal facts reader requires qualification and flag plus
the measured Gemini Flash High profile: exact32768 output tokens, actual timeout
at least300s and a matching durable budget quote. Doctor reports that profile
prerequisite separately. Typed owner metadata operations do not perform inference
and retain their independent owner/feature authorization. Deterministic expiry
filtering of accepted profile/episode context survives disabling the temporal
feature or changing the provider. The feature gate controls new temporal
consumers and context annotations; it cannot revive an expired exception.
