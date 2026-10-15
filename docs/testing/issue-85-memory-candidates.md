# Issue 85: source candidate consumer

The complete version2 native screen passed its prospectively fixed criteria on
the actual High subscription transport. The code-owned production release gate
remains false pending final source/profile review; operator flags alone cannot
activate it. This report distinguishes controlled composition, native extraction
quality, and owner-controlled acceptance of a native result.

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
not acceptance. The later independent
[`TestActualPostgresCandidateEpisodeDumpRestoreDeletionIntegration`](issue-85-episode-backup.md)
gate proves the new accepted-episode projection across real old dump/fresh restore,
newest independent deletion-anchor replay twice, physical erasure, resurrection
denial, unrelated accepted-episode survival and unchanged paid history. Its
three explicit phases plus eight regression test/subtest events passed 11/0/0
under race and vet (`5bca2f875a5ae1f914af1b3e461579304301b6fb`). Whole source
manifest SHA256 is `447daad1d4cac9c5dce29c75748a847f1c566fa77fd60e0c28c655454de8c2e1`;
evidence manifest is `a305891064fde6b90f758cf7cc32e7f4caa87ac6ca74dd12013f3ebd7da6e3b1`.
It uses controlled provider outputs and establishes privacy/lifecycle behavior,
not native episode extraction quality.

`TestCandidateAcceptedPreferenceScopedOrdinaryContextIntegration`
(`5253a2339ba8ee081bda7ae047435fc0fb7fc9cf`) separately crosses authenticated owner
policy, worker publication, HTTP owner acceptance and ordinary task execution.
The accepted quoted applicability appears in the owning session's ordinary
context and is absent in another session; correction and source revocation remove
it from subsequent ordinary context. Actual PG/race passed in 3.88s, with independent
review. This controlled source/context proof does not claim native answer quality.

The full jobs/catalog/candidates race regression passed on `ssh laba` after the
shared supervisor fix (`supervisor-regression-v3.jsonl`, jobs 79.914 seconds).
The final supervisor mutation/repro set passed in 18.586 seconds. Its old full
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
race set then passed in 18.586s. Independent review found no remaining authority,
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
Declared subscription-no-overage cost is 0 micros; subscription allocation,
actual HTTP egress/OAuth bytes and CPU/storage amortization are unmeasured.

## Version2 native qualification

The general metadata instruction changed to `source-candidates-v2`; it clarifies
quoted scope applicability, exceptions and validity markers without literal
examples from the failed core cases. Before any version2 call, an independent
eight-case holdout was committed (`f8a03ec7`): new project names, reordered
applicability, future/relative dates, negated exceptions, quotation/conditional
ambiguity and unknown fields. Its semantic threshold is100%; the original 24-case
core threshold remains95%.

Prompt SHA256 is
`b03f07b7d6882d54e4968a428467a4db0bfb7fb3d3ac09d60ca8f4f8fbee2c8d`;
holdout SHA256 is
`ac46c92c1f1c628203dd420069d8647021ca02da97d62bf2334547d01b6d2201`.
The prompt, both corpora and thresholds stayed unchanged throughout all version2
windows. Every new screen used new one-use policy/source identities and retained
all earlier attempts, unknown usage and conservative reservations. Auth-only
diagnostics used no model turns. The existing High subscription transport ran
only through the server's18080 proxy.

| Window | Frozen source/profile | Actual attempts | Result |
|---|---|---:|---|
| native-v4 | `4e521b06`,8192/90s | 26 | Core23/24; first holdout passed; second holdout `provider_unavailable`, unknown usage; six unattempted |
| native-v5 | `d4baa269`,32768/300s planned | 0 | Typed policy-control fixture used `id` instead of `policy_id`; failed before admission;50 old attempts/reservations unchanged |
| native-v6 | `85952396`,32768/300s | 32 | Core23/24, holdout8/8, citation32/32, unsafe0, authority0, window unknown0: PASS |

The version4 failure at 10.061s had no failing PostgreSQL deadline/probe. The
credential-discovery helper has a 10s deadline and its caller maps failures to
`provider_unavailable`, but the historical attempt lacks stage diagnostics; a
credential timeout is a hypothesis, not an established cause. A subsequent
serialized existing `diagnose-auth`/`diagnose-cloudcode` preflight resolved safely
and completed discovery with HTTP 200. That proves current preflight health only.
No historical unknown spend was cleared or described as zero.

The larger tested profile reserves 32768 output tokens and 300000 active milliseconds
per call. The actual worker claim lasts 6m and its whole-attempt context allows 360s,
so an old 110s outer context cannot silently truncate the 300s model allowance.
Independent actualPG tests inspect the effective context without waiting 300s.
Before any fresh call, the harness checks the complete finite window against
the existing owner-day ledger and explicitly revokes old policies through owner
HTTP controls. Version5's setup defect was fixed and tested against realPG;
the consumed window label was preserved rather than reused.

Version6 prospectively capped 32 calls, 34,603,008 reserved tokens, 9,600,000ms and
268,435,456 provider-input bytes, with a 3h expiration. All 50 earlier attempts
were already in the same durable database. The existing 14,400,000ms owner-day
limit was not reset or raised. The complete screen passed in 339.14s; every call
reported known usage and allowlisted finish reason `STOP`. Every captured
subscription discovery completed with HTTP 200. The content-free PostgreSQL trace
shows full transaction commits taking 359/484/1028ms but no failed observer deadline;
the durable full proofs kept those commits while the read-only periodic observer
used bounded rollback. Benign observer cancellations after provider EOF remain
visible. These observations do not imply all SQL stayed below 200ms.

The one semantic miss in both version2 core screens is `unknown-time`: the exact
source `Мой основной редактор — Neovim.` is expected to be a scoped fact but the
model labeled it an explicit preference. Quote, nullable time and actual scope
were preserved. This remains a real type error; 23/24=95.83% meets the unchanged
core 95% criterion. Holdout 8/8 meets its separately fixed 100% criterion. No prompt
or expected answer was changed to remove this error.

After version 6 passed, the separate owner stage passed in 0.97s. It used one actual
native-produced preference candidate through authenticated HTTP acceptance,
confirmed catalog memory eligibility, source correction/invalidation and explicit
source revocation. These intentional owner writes are recorded separately from
extraction's zero-authority criterion. Additional model calls were 0; paid attempts
were 82 before and after. Native accepted episodic backup behavior is not inferred
from this preference stage; the independent controlled episode gate above covers
that projection's lifecycle.

## Full retained cost and release scope

| Spend scope | Attempts | Known / unknown | Known input / output / total tokens |
|---|---:|---:|---:|
| Original version1 | 24 | 23 /1 | 14,662 /2,448 /48,923 |
| Version2 native-v4 | 26 | 25 /1 | 21,404 /2,668 /57,556 |
| Accepted native-v6 | 32 | 32 /0 | 27,379 /3,482 /70,074 |
| Cumulative | 82 | 80 /2 | 63,445 /8,598 /176,553 |

Cumulative conservative reservations are 87,441,408 tokens, 14,100,000 active
milliseconds and 687,865,856 provider-input bytes. The remaining owner-day active
allowance is 300,000ms. The two historical unknown usages remain unknown and fully
reserved. Totals include provider thinking and therefore exceed visible
input+output. Each attempt is counted once; the same operation appears in several
budget scopes and must not be summed repeatedly. The declared subscription
no-overage amount is 0 micros, not zero total cost. Subscription allocation,
actual HTTP/OAuth bytes and CPU/storage amortization remain unmeasured.

Safe frozen caps, binary/source hashes, raw synthetic observations, full ledgers,
PG diagnostics and owner receipt are committed in
[`deploy/memory-candidates/20261016`](../../deploy/memory-candidates/20261016/README.md).
Original binaries, durable database, logs and encrypted auth artifacts remain in
the isolated server directory; credentials are excluded from the report.
The legacy `controlled-summary.json` counts 196 pass records across overlapping
controlled runs, not 196 distinct tests; its two optional skips are not acceptance.
The later native and explicit real backup procedures above supply their own proof.

`candidateProfileReady` binds release eligibility to the actual subscription
Antigravity High provider with `InferenceLimits` output exactly 32768 and budget
OutputTokens at least 32768/ModelMillis at least 300000. Both model-jobs and
candidate environment flags must also be true. Default 8192, unmeasured 65536,
another provider/model, absent limits and under-reserved budget all fail closed.
The code-owned qualification constant is still false pending a separate reviewed
release decision; the frozen screen accurately records that historical state.
Production flags and the main deployment were not changed during these tests.

This finite screen establishes the stated extraction criteria for this profile
and synthetic Russian corpus, not broad extraction or native answer quality.
Quoted applicability remains source data. Broader issue 77 readiness also depends
on the other consumers and their applicable gates.
