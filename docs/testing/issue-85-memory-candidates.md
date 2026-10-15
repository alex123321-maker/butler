# Issue 85: source candidate consumer

The production consumer is implemented but its code-owned release gate remains
false pending the complete consumer quality/lifecycle qualification. An environment
flag alone cannot activate it. This report distinguishes controlled composition,
native extraction quality, and owner-controlled acceptance of a native result.

## Mechanism

`source-candidates-v2` uses the existing durable generative worker, subscription
provider transport, PostgreSQL reservations, immutable source manifests, current
typed policy/assignment and per-HTTP authority/dispatch guards. The model receives
only the selected original leaf; earlier transcript events and document parents
remain complete privacy dependencies. Output creates confidence-zero candidates,
never accepted profile/episode state. Existing authenticated owner decision/CAS,
correction and deletion APIs remain the authority boundary.

Exact Unicode sentence evidence includes original revision/hash, rune offsets,
actual source scope, event/ingestion/extraction time and nullable quoted temporal
applicability. Typed explicit preferences/scoped facts project to Profile Memory
and outcome episodes to Episodic Memory only after owner acceptance. Dedupe uses
the original evidence anchor and refuses a changed interpretation on the same
anchor; repeated policies/restarts cannot reset an owner decision. Local-only
policies defer without cloud requests or reservations.

## Controlled composition

Actual PostgreSQL, race-enabled tests cross authenticated HTTP owner controls,
durable policies/jobs, the worker, controlled provider boundary, atomic candidate
publication, owner projection and correction/deletion. Relevant tests are:

- `TestCandidateHTTPWorkerOwnerDecisionCorrectionIntegration`
- `TestCandidateTranscriptAndParsedSourcePreserveAncestorPrivacyIntegration`
- `TestCandidateLateSourceChangeAndLocalOnlyIntegration`
- `TestCandidateEpisodeOwnerProjectionAndRevokeIntegration`
- `TestCandidateAtomicConsumerRollbackIntegration`
- `TestCandidateLifecyclePoisonRemainsDataAndDeletionSelectiveIntegration`
- `TestCandidateSeparateOwnerLifecycleStageIntegration`

The lifecycle consumer test uses all eight independently shared #83 attacks. It
checks data-only candidate authority, exact source provenance, late correction/
deletion rejection and selective deletion preserving an unrelated accepted claim.
Native inference is separately opt-in; missing PostgreSQL/native flags are skips,
not acceptance. Backup restore is not proved by these candidate tests.

The full jobs/catalog/candidates race regression passed on `ssh laba` after the
shared supervisor fix (`supervisor-regression-v3.jsonl`, jobs 79.914 seconds).
The final supervisor mutation/repro set passed in 11.963 seconds. Its old full
`CheckObjects` probe demonstrably waits behind the same job's publication row
lock and expires at 200ms; the metadata probe passes under that same lock and
deadline. A session-scoped publication holding policy/session/job locks for
800ms also passes because the in-flight observer stops and joins at provider EOF.
Full admission, network release/dispatch and publication proofs remain intact.
Direct Secret sensitivity and required summary-original metadata are included.
The proof deadline, cancellation and conservative unknown-cost treatment were
not relaxed. A separate warm-two-connection experiment by #84 reproduces the old
failure; connection warming alone is insufficient. Historical native failures
lack a precise old SQL wait trace, so this repro is not a claim that every old
timeout had the same exact statement as its cause.

The native transport repeats full authority after credential resolution. A
per-job proof mutex now lets a periodic tick yield only while this same mandatory
pre-HTTP authority/dispatch proof is running. There is no new HTTP dispatch
without that full proof, and the original call deadline remains active. An actual
PG fixture blocks the second authority check on a session for600ms: unchanged
authority publishes after one simulated HTTP; worker stop, policy revocation,
global disable and source-access change all deny before zero HTTP. This passed
under race in10.38 seconds. A phase observer identifies begin/restore/policy/
claim/sources/rollback failures without retaining raw SQL, parameters or errors.

A subsequent #84 native trace isolated a separate timeout at the observer's
literal `commit` command (196ms), with an idle second connection; this was not a
pool-acquisition or source-query timeout. The observer has no business/audit
mutation to commit. Its audited helper chain therefore now ends with bounded
rollback; full network/publication commits are unchanged. A PG test verifies that
job/policy/control/attempt/source/task/goal/mandate/reservation rows are unchanged
and no commit is sent. With an artificial300ms commit delay it passes in0.50s;
the same test on the previous commit-based observer fails at its200ms deadline.
This control demonstrates avoiding an unnecessary durability boundary, not a
reproduction of a particular physical disk/fsync fault. The full supervisor
race set then passed in18.586s. Independent review found no remaining authority,
privacy or transaction-boundary issue in this change.

## Retained source-candidates-v1 native history

The fixed original corpus is `ru-memory-candidates-v1`, SHA256
`902efbad59a6a11f39521d5c3cd5e4be91a10f13ef23e4c7df2570063d6be0cd`.
Criteria were declared before inference: semantic cases at least95%, exact
citations100%, unsafe promotions0, authority writes0, current-window unknown
usage0. The original thresholds and cases were not edited after inference.

| Window | Frozen source | Actual attempts | Result |
|---|---|---:|---|
| native-v1 | setup | 0 | Wrong isolated PG socket port; no inference/reservation |
| native-v2 | 66177be2 | 3 | First2 passed; third storage supervisor timeout, usage unknown |
| native-v3 | ff6fb8fd | 21 | Exact original remainder;21 known/structural,19 semantic matches |

The two observed semantic misses were exact quotations with omitted metadata:
`before-now` omitted an explicit current-time marker, and `episode-success`
omitted the named project scope. These are real failures, despite exact source
citations. The24 original attempts therefore did not qualify this version:
21/24 semantic successes,23/24 observed exact structural results,1 unknown.
No unsafe promotion or automatic authority write occurred.

The persistent test database retains all24 attempts and reservations. Reserved
cost is25,362,432 tokens,2,160,000 active milliseconds and201,326,592 provider input
bytes. Known observed usage is14,662 input,2,448 output and48,923 total tokens
(total also includes provider thinking). The one unknown attempt is not zero.
Declared subscription-no-overage cost is0 micros; subscription allocation,
actual HTTP egress/OAuth bytes and CPU/storage amortization are unmeasured.

## Fresh version2 qualification

The general metadata instruction changed to `source-candidates-v2`; it clarifies
quoted scope applicability, exceptions and validity markers without literal
examples from the failed core cases. Before any version2 call, an independent
eight-case holdout was committed (`f8a03ec7`): new project names, reordered
applicability, future/relative dates, negated exceptions, quotation/conditional
ambiguity and unknown fields. Its semantic threshold is100%; the original24-case
core threshold remains95%.

The fresh window must run all32 cases under new one-use policy/source identities,
with all earlier spend carried forward. No previous attempt is reset, refunded
or silently replayed. Its finite prospective reservation is32 calls,
33,816,576 tokens,2,880,000ms and268,435,456 provider input bytes. The existing
High subscription transport runs only through the server's18080 proxy.

After a passing extraction screen, a separate zero-model-call owner stage uses
one actual native-produced candidate through HTTP acceptance, catalog memory
eligibility, source correction/invalidation and explicit source revocation.
Intentional owner writes are recorded separately from extraction's zero-authority
criterion, and paid attempt history remains unchanged.

Fresh window results and the final release decision are pending. Passing this
finite screen would not establish general extraction quality or final task success;
broader #77 readiness also depends on its other consumers and applicable gates.
