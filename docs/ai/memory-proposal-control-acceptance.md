# Ordinary agent candidate publication —2026-10-11

Related #17. Default-false `BUTLER_MEMORY_CANDIDATE_ENABLED` in both services
composes canonical `memory.propose_candidate` revision1 and private owning
`/internal/memory-candidate`. Model arguments contain only bounded source/revision,
fact key, JSON value string and summary. They cannot set owner/scope, confidence,
acceptance, deduplication identity or execution authority. Confidence0 remains
DATA. The source must already be pinned by the actual run and independently
accessible for extraction. Original source/ancestor revisions remain current.

The current accepted-input claim is pinned privately in the grant while the
approved action hash remains immutable. Owning publication reconstructs Session
proof from durable state and checks exact input dispatch slot, task/assignment/
constraints, original sources and permanent Broker dispatch in the same PG
transaction before and after publication waits. Catalog deduplication uses the
actual effect ID. Current owner head CAS remains the only acceptance mechanism.
Recognized credential keys/patterns deny the proposal without rewriting intent.
The fixed private client denies ambient proxies, redirects and alternate routes.

Actual disposable PG17/pool2 tests cover 15 owning dispatch scenarios: proposal,
exact retry, actual Broker application subprocess (`App.New/Run`), lost ACK, unclaimed
dispatch, unpinned source, changed effect/body/contract, wrong input slot, lost
input claim, expired input/lease, changed task and deleted source. Anonymous
Broker execution is denied. Success proves exactly one candidate and one shared
budget reservation, zero accepted profiles, source-derived owner/scope and fixed
confidence0. Owner actor substitution is denied. Private protocol tests reject
ambiguous fields/authentication, authority fields, accepted receipts and redirects.

The actual Broker application's lost-ACK test commits the candidate, drops the
private HTTP response, verifies `effect_unknown`, and submits the same Broker
call again. The owning endpoint was called only once, with exactly one candidate
and shared budget reservation. A separate actual dedup-lock wait crosses the
original input-claim deadline while the run lease remains current; publication
denies and rolls back both the candidate and deduplication row. The observer
refreshes PG activity snapshots and proves this exact blocker holds the lock.

Two current independent Session workers concurrently propose conflicting values
for one accepted user preference. Both results remain candidates and the accepted
head stays unchanged. Worker acceptance is denied. Two competing owner decisions
with the same expected current head yield exactly one accepted version and one
stale decision. There is no last-worker-wins update. This is a catalog boundary
proof; actual model extraction and human review quality are separate.

Final code snapshot:
`/srv/storage/butler/agents/memory-proposal-control/snapshots/20261011-181436-409`.
Expanded report:
`/srv/storage/butler/agents/memory-proposal-control/reports/memory-candidate-20261011T111554Z-797870/tests.jsonl`.
SHA256 `1bf028a6a35d6779971bcf96294ca72db0ecc5bd3b77a56db7975c5ac4a43c3b`.
415 distinct pass names/9 packages, race detector and vet passed, zero model calls.
Seven separate tests skipped: opt-in dump/restore, raw browser e2e, two standalone
delegation harnesses, controlled/guarded Chrome, standalone execute/transcript.
Their acceptance is not inferred from this run. The actual candidate Broker app
case passed in1.67s, lost ACK in1.69s and input-expiry wait in2.53s.
The private disposable PG/network were cleaned up.

Earlier narrow proof `memory-candidate-20261011T110113Z-749304` passed with hash
`ab2e249bd4a76c8e7825e9c8d8c3a3724a6f75b59e3ac9c4fc076e7655db6452`.
The pre-fault expanded proof `...T110353Z-759114` passed 413 names with SHA256
`ae5730c3b8b291645cada08100560aafda085e90d0f5f8749423d20786dfde6f`.
Final narrow proof `...T111445Z-793525` hash is
`1c0d5eac928e74fe01dbffccdd1a9cc3a1ff1ae8986c63f89ac75102598ffa0d`.
Initial compilation failures at `...T105921Z-742773` (unused import) and
`...T110311Z-756469` (wrong test fixture name) remain retained. Neither is a
successful proof, and no checker or authority condition was relaxed to pass.
`...T111125Z-782909` retains the failed wait observer with cached PG query text;
the lost-ACK case passed there, but the report as a whole was not a passing proof.

Orchestrator's owning service, actual private action reader and publication API
use current PG state in the fixture; its complete Redis/provider bootstrap and a
human/model-driven conversation were not started. Main laba deployment/flags were
unchanged. This does not enable the legacy cloud pipeline, prove local-model
quality, broaden context-only child permissions or close #17's remaining workspace
and product/native acceptance gates. Lost ACK retains unknown Broker effect state;
it does not authorize an automatic replay or acceptance.
