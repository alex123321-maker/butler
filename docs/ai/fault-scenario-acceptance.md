# Issue #33 scenario evidence and remaining acceptance

Current2026-10-13 evidence supersedes the historical status table below:
[actual V5 High quality/comparison](scenario-cloud-v5-independent-audit-20261013.md)
passed236 Russian quality cases and all four paired tasks with complete new cost;
the same final source's mandatory matrix passed321 names without fail/skip.
The [actual native V5 chain](scenario-release-chain-v5-independent-audit-20261013.md)
is sealed NO_GO at537 cumulative admissions: the second entry timed out with
unknown cost before delegation, adapter development, result and notification.
Full issue acceptance is therefore still NO_GO; PR74 remains draft and issue33
open. No historical component-only/model-absent statement below describes this
current complete set of evidence.

The executable matrix is `internal/acceptance/matrix.go`; its contract and
reproduction instructions are in `internal/acceptance/README.md`. It distinguishes
available deterministic core boundaries, actual Chromium/private service paths,
actual backup/restore, and missing product scenarios. A missing or skipped required
test fails. No final model response supplies permission or verification evidence.

The fixture effects and credentials are synthetic. Disposable databases and an
internal network belong to one lab invocation; main Butler state, shared Compose,
Google credentials and live proxy are not used. The operator's original persistent
deletion anchor is not mounted or changed. The backup proof creates a **new**
independent anchor, saves PG before deletion, commits deletion, restores the old
dump into a fresh test DB, demonstrates fail-closed access, reconciles the newest
anchor, proves original/derived text and binary erased, and rejects reindex.

The new `TestApprovalWorkerSIGKILLRestartIntegration` launches a separate worker
process against real PG, waits for a committed approval checkpoint, verifies actual
Linux SIGKILL, resolves and repeats the owner's response, then constructs a fresh
worker with a two-connection pool. It preserves run/tool IDs and records one
controlled effect with a fresh Butler-owned provider context. This is an OS
process-death proof; it does not claim a real cloud provider or arbitrary tool
receipt. Other faults remain precisely labelled replacement-Service/checkpoint,
lost external acknowledgement, transaction-order or controlled-worker faults.

Initial standalone runs correctly found infrastructure and product failures:
a 256MiB disposable PG tmpfs ran out of space; its remaining checks were FAIL.
The runner now keeps PG data under its private storage directory. Race-enabled
actual Chromium exposed `ScopedProxy.timer` initialization versus expiry `Close`;
commit 602fca9 protects initialization with the existing mutex and its independent
expiry regression passed on the lab. The credential fixture now initializes a
private deletion anchor and pins its actual transcript before the real Broker,
matching the mandatory memory barrier; it does not disable that guard.

| Criterion | Current evidence / limit |
| --- | --- |
| Reproducible controlled effects | Fixed lab scripts, disposable PG/Redis, exact Go events, bounded actual Chromium and private Broker/HTTP/approval fixtures; see final run record below |
| Initial Russian data | 240 self-authored cases / 30 source groups, 60 each retrieval/extraction/classification/commands; no real personal data or working secret |
| Development/tuning/holdout separation | 96/64/80 cases, 12/8/10 groups; original and all derived cases stay in one split; exact/near-trigram screening and independent V3 golden review passed; public synthetic data are not a hidden benchmark |
| Critical contracts | Named Go assertions, real PG race/order tests, actual SIGKILL, Chromium and dump/restore; no model judge required |
| Model outcomes and resources | Strict observation scorer with exact source/command schema, classification errors, measured latency/tokens and pinned weights/runtime/hardware; actual observations absent, NO_GO |
| Single versus multiple agents | Same-task/model/code/hardware/resource/pricing validator and complete per-task cost fields; actual experiment pending #16/#17 |
| Independent golden control | Separate user-appointed critic signed frozen V3 corpus+criteria; outside-source key/approval and pinned verifier verified; automatic protected release composition remains pending #20/#21 |
| Reproducible version pins | Exact code commit and snapshot hash, migration/corpus/criteria hashes, Go/container IDs, hardware manifest; raw private artifacts retained on lab |
| Observation before model release | Export/scoring cannot execute an action; secret sources excluded before input; no model connected for this corpus and NO_GO cannot promote one |

The current matrix is based on published main release
`dfff7f7fc8ee6c8ad23fe093175b85a6d2b4e041`, which combines #18 and the
memory/ingestion/source guards. It requires exact installed-version/readiness/
revocation PG tests and the actual separate Broker/extension runtime fixture with
current transcript pins and private deletion anchor. An older snapshot without
the installed package reports PENDING. Isolated adapter development with an
independent reviewer, and self-repair release, still require #20/#21.

**Issue #33 remains open.** Required #16/#17 real delegated tab/file conflicts and
child provenance, #19 reusable procedures, #20/#21 isolated development/repair with
independent controls, and the full #15-to-single-notification chain across restart,
stop and delivery failure remain PENDING. Controlled pressure fixtures do not
establish actual model OOM/latency/resource accuracy. Corpus preparation and a
perfect scorer unit fixture are not model quality evidence.

## Recorded lab runs

V3 received **PASS_GOLDEN_REVIEW_ONLY** from the independent user-appointed critic
on frozen commit `159934737a42eeec7a1deef2b90f6a9a6fd4c0b1`. All 240 cases were
reviewed, ten harness tests passed with race/vet and 76 independent malformed
measurement checks were rejected. The signed approval and public key remain
outside candidate source in `/srv/storage/butler/reviews/issue33-20261011-r3`.
The [content-free verdict](../../testdata/acceptance/reviews/ru-v3-golden-review-20261011.json)
pins their exact hashes, corpus, criteria, reviewer and separately frozen binary.
The reproduction helper successfully verified the signature after review.
Detailed review SHA256:
`2074d86dd88589cf1bf4c89e0b689c56dc55e3aadbc1bc3fb219e7c1bfa0963a`.
This is role independence on a shared host, not administrator isolation. It
authorizes no execution, model release or quality claim; actual model calls
remain zero. Prior NO_GO files and corpus versions remain unchanged.

The integrated Full matrix on the same frozen commit also passed 28 groups and
137 required executions, with eight scenarios PASS and five PENDING. Its private
proof is `/srv/storage/butler/agents/issue-completion/reports/fault-20261011T052708Z-3845374`;
the [integrated content-free report](../../deploy/security/acceptance-integrated-20261011.json)
has SHA256 `05f587f517e1c21f459465ec2eef58b7c070c98ead52bd22947bde9654ce66bf`.
`issue_33_ready=false` remains unchanged by the golden review.

The separate second review rejected V2 with NO_GO. Its
[content-free verdict](../../testdata/acceptance/reviews/ru-v2-no-go-20261011.json)
and detailed external report remain at `/srv/storage/butler/reviews/issue33-20261011-r2`
(review SHA256 `a199edf4b46f7df793a93f21cab0bed3bce43af902dbff83890afe57fdb9a6dd`).
V2 fixed attack-family leakage but left additional negative-search and
draft-only ambiguities, a negative extraction header containing RetryAfter,
and absent metrics decoded as zero. Active V3 changes all absent questions to
undocumented concrete properties, excludes target values from negative injection
documents, clarifies draft-only requests, and strictly requires every measured
counter. Unknown usage cannot become zero. V3 golden approval subsequently passed
as recorded above; actual model measurements remain pending and earlier
reports/verdicts are immutable.

Independent reviewer `codex-independent-corpus-critic-20261011-r1` rejected V1
with NO_GO. The [original verdict](../../testdata/acceptance/reviews/ru-v1-no-go-20261011.json)
is retained with unchanged V1 corpus bytes. The detailed independent report is
outside candidate code at `/srv/storage/butler/reviews/issue33-20261011/review.md`
(SHA256 `765edbc778f7b39e1696f778149bb8a3f74b539f90a4eff908d7b19bf14119da`).
No signature, key or approval was created for rejected V1. Active V2 addresses
the three findings with seven undocumented-fact questions, 15 split-confined
attack templates (including mixed legitimate facts), and defined command
classification rules. Its public synthetic templates are authored by the
implementation agent; this does not make the golden answers independently
approved. Independent re-review and actual model measurements remain pending.

The later integrated Full run on `7b1e1e2bd703d33357a9b1ce0bd420f2813eef13`
passed 28 groups / 137 required top-level executions, with eight scenarios PASS
and five PENDING. It makes programmatic security/provenance tests mandatory,
including child source grants and fixed transport failure diagnostics. The
[content-free report](../../deploy/security/acceptance-20261011.json) and
[security audit](security-acceptance.md) pin exact evidence. Actual model calls
remain zero, `selected_passed=true`, `issue_33_ready=false`. Private proof:
`/srv/storage/butler/agents/issue-completion/reports/fault-20261011T043536Z-3667778`.
The older runs below remain their original measurements.

The final Full run passed **20 groups / 104 required top-level executions** on
the lab, including 13 core Go/PG/Redis groups with race detection, actual
Chromium, Broker/credential and installed extension processes, and the three
actual dump/delete/restore phases. These are 104 executions (the same restore
test is required in each phase), not 104 independent statistical observations.
Seven available scenarios passed; six required product scenarios remain PENDING.
`selected_passed=true`, `issue_33_ready=false`, model status `NO_GO`, actual model
calls **0**. The harness's own policy/scorer tests also passed under `-race`.

The content-free record is
[`lab-release-20261011.json`](../../testdata/acceptance/lab-release-20261011.json).
It pins tested commit `189320fb8b789c0c4cb76fa0a0f2cc5c70cd259b`, source snapshot
`20261011-065339-408`, source/schema/corpus/criteria digests, recorded container
and hardware identities, per-group event hashes, and the genuine backup hash.
Private artifacts remain at
`/srv/storage/butler/agents/durable-tasks/reports/fault-20261010T235341Z-2382388`.
The observed host has 12 logical CPUs and about 15GiB RAM; tests reserve bounded
container allowances. Stored Docker stats are a point-in-time sample, not a peak
measurement or model cost. No hardware OOM or model accuracy is inferred.

The earlier release-snapshot Full also passed at
`fault-20261010T235103Z-2366627` (commit `9d03de6`, 19 groups, before the separate
extension-process group). Earlier failed reports remain in the agent root and
are never overwritten or relabelled PASS. Private raw events, PG data, dumps and
anchor contents are not committed to the repository.
