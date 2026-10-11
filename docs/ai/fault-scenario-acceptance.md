# Issue #33 scenario evidence and remaining acceptance

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
| Development/tuning/holdout separation | 96/64/80 cases, 12/8/10 groups; original and all derived cases stay in one split; exact/near-trigram screening runs programmatically; independent semantic review pending |
| Critical contracts | Named Go assertions, real PG race/order tests, actual SIGKILL, Chromium and dump/restore; no model judge required |
| Model outcomes and resources | Strict observation scorer with exact source/command schema, classification errors, measured latency/tokens and pinned weights/runtime/hardware; actual observations absent, NO_GO |
| Single versus multiple agents | Same-task/model/code/hardware/resource/pricing validator and complete per-task cost fields; actual experiment pending #16/#17 |
| Independent golden control | Ed25519 reviewer/candidate separation, corpus+criteria digest, outside-source public-key/approval mounts, no signing command; no approval supplied, and automatic release composition pending #20/#21 |
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
