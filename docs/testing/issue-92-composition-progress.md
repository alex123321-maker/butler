# Memory composition evidence and remaining gates

This is controlled component/composition evidence, not completed issue77/92
acceptance or a production activation. The primary laba deployment remains
unchanged. Synthetic data is not human-reviewed real user workload.

The [staged rollout contract](issue-92-staged-memory-rollout.md) defines separate
entry/exit criteria, owner review, effective flags and compatible rollback. It
does not claim that any pending stage has run or activate the primary deployment.

The controlled rows below ran on `ssh laba` with cached Go1.27.2/modules, actual isolated
PostgreSQL and race detection. No cloud credentials or inference were mounted.
The original versioned windows and their failures remain retained separately.

All coherent foreground receipts before v10 have an inherited-owner fixture
limitation: they establish the recorded physical mechanics, not ordinary-worker
authorization or final publication. Neutral-worker v10 failed after19 checkpoints.
The latest static neutral-worker lifecycle-v12, source30939510, passes all24
checkpoints and47 test/subtest events with race/vet. The separate effective-v2
constructor/Doctor delta passes79 events,0 failures/skips. The broader
composition-v7 at sourcee09638dd passed464 events,0 failures/skips, race and vet,
including the mandatory immutable-history fixture; its receipt predates the
bootstrap-only delta. Earlier table rows retain their original limits.

| Source / window | Observation | Limits |
| --- | --- | --- |
|0861c67d / composition-v1|100 test/subtest events passed,0 failed,0 skipped; vet passed|Includes88/89/91 and shared constructor receipt, before90 integration and079; not a final current-version pass|
|10ebb429 / privacy-v1|4 mandatory tests passed,0 failed/skipped; vet passed|Actual ordinary Broker raw/view reads followed by deletion, cached tool/approval erasure, unchanged full action/evidence/spending digest; current-schema upgrade fixture; ordered077/079 migration test|
|3b24b6eb / payload-backup-v1|All3 mandatory phases passed: actual custom pg_dump before deletion, deletion, fresh pg_restore plus newest independent anchor|Synthetic tool-cache pins are explicitly seeded; actual Broker capture is proved in privacy-v1. Unrelated original/accepted fact/tool cache and actual UNKNOWN status survive; repeated reconciliation is idempotent|
|9aac9061 / composition-v2|178 test/subtest events passed,0 failed/skipped; race/vet passed|Combined83–91 controlled subset with077/079, current actual Broker and recipient/extension/HTTP binaries; before the subsequent source-observer rollback/accounting patch; final ordered pilot still required|
|2f8a9d22 / ancestry-facts-v2|19 test/subtest events passed,0 failed/skipped; race/vet passed|Current077 ancestry plus079; actual Broker reads, accepted temporal snapshot restrictions, unknown/mismatched environment and negative feedback, exact exposure subset, reviewed skill lineage and untitled prompt regression|
|530493b3 / composition-v3|224 test/subtest events passed,0 failed/skipped; race/vet passed|Adds source-observer rollback, known-usage suppression/ticker/run-binding cases, actual model job priority and persistent90 window controls; before later startup profile, temporal interpretation and080 changes|
|c0a1450a / ancestry-facts-v3|19 test/subtest events passed,0 failed/skipped; race/vet passed|Adds the actual Broker temporal-facts path through an accepted derived lesson with its current environment proof|
|470765fa / privacy-v2|4 mandatory tests passed,0 failed/skipped; race/vet passed|Includes refusal to downgrade079 after a durable run payload erasure marker and retained denial of late raw cache restoration|
|08ca8456 / temporal-context-v2|6 mandatory tests passed,0 failed/skipped; race/vet passed|Accepted profile and exact/semantic episode expiry survive disabling annotations; actual facts Broker retains fresh versus unknown verification basis. Controlled interpretation contract2, not native answer-quality acceptance|
|7294d7ef / composition-v4|237 test/subtest events passed,0 failed/skipped; race/vet passed|Adds the local-index observer rollback fix, mandatory temporal expiry/facts-basis controls and actual90 original-task runtime foundation; before080 terminal-attempt integration and the trusted single-proposal instruction|
|8934b67a / background-v2|2 mandatory tests passed,0 failed/skipped; race/vet passed|Ordinary foreground completed257ms during actual compaction and273ms during actual consolidation; each original NULL-usage quote survives unchanged, and a separately predeclared charged retry finishes the consumer|
|c803dd0e / coherent-privacy-v1|1 mandatory test passed,0 failed/skipped, race;5.53s|All15 earlier shared-timeline checkpoints followed by late owner deletion after actual prepared model input; final blocked, prior private copies/summaries erased, known usage and effect journal retained. Still a partial controlled pilot|
|b3be3a12 / composition-v5|259 test/subtest events passed,0 failed/skipped; race/vet passed|Includes080 terminal-attempt provenance/currentness/owner-only policy and the trusted single-proposal instruction. This controlled combined subset is separate from native quality and the complete92 pilot|
|8df1331b / coherent-experience-v1|1 mandatory test passed,0 failed/skipped, race;7.93s|The same15-checkpoint timeline continues through an actual Broker/extension/HTTP original, independent artifact verification and owner-accepted episode,17 sequential checkpoints; controlled outputs, no native inference|
|fdc96d54 / coherent-lifecycle-v8|18 test/subtest events passed,0 failed/skipped, race|All23 sequential same-state preference/verified-episode/consolidation/deletion/custom-restore/latest-anchor/real-ENOSPC checkpoints, plus mandatory port/dependency/erasure/contract controls; native/auth0, controlled outputs|
|288b7ea4 / coherent-lifecycle-v9|19 test/subtest events passed,0 failed/skipped, race/vet|All24 same-state checkpoints including actual Broker/runtime UNKNOWN/no-final/no-replay through consolidation/delete/fresh restore; complete effect/evidence/budget digests unchanged, actual ENOSPC; controlled outputs, native/auth0|
|7bac6cda / interactive-v5|Ordinary Flow completed370ms with one actual local query/index IPC overlap;1PASS,0FAIL,0SKIP with race|100 original vectors plus100 new fragments; controlled foreground provider, zero cloud calls; not p95 or full mixed-model workload|
|8ed0770f / disk-v1|Actual ENOSPC, test0.68s, passed without skips|Dedicated8KiB tmpfs, one synthetic schema; unchanged PG deletion state on failed append. Observed partial anchor becomes unavailable and explicitly denies reads/reconciliation; no automatic reset or claimed deletion|

Private reports: `/srv/storage/butler/agents/issue92-20261016/reports/`.
The temporal-context-v2, background-v2 and coherent-privacy-v1 reports are under
the corresponding `issue92-20261018` root.

| Artifact | SHA256 |
| --- | --- |
|composition-v1/tests.jsonl|3fe029067e3c4d0945f232e02e165f55385cccf5b01c525b103274942a1133ff|
|privacy-v1/tests.jsonl|be23ddd37569b4a677a962f2540d7db2fce322b16cf8bba93a4fd706f2d3aac1|
|ancestry-facts-v2/tests.jsonl|547b876a6f90a5f47b166ce4edab17fc9b278a10ef331bddd09aa3534102f4a9|
|composition-v3/tests.jsonl|b7453ac61c3c189dd504e384a56aa6f79948b52193647858db108c55df039841|
|ancestry-facts-v3/tests.jsonl|906298f2746676529c648912e2ff1319667a616de252bcb8940ea9bf9c6245bc|
|privacy-v2/tests.jsonl|a9d380fb59359f508ba4d5e60be98c7eaa0264aa516c912fe2c4556602241f3c|
|temporal-context-v2/tests.jsonl|f4731aeef90afd47cf50f2cc4ebee615f4e5222a44da6db0ad6035089144448f|
|composition-v4/tests.jsonl|59842cfffb58834c2a7b2b7ab55e317477cd9c53b4bfe61fc642ede750001eea|
|background-v2/tests.jsonl|1c3e6b5903c486e1b09c1af96568072739f2961581b45c6b835ff6fa9cffec07|
|coherent-privacy-v1/events.jsonl|e1bf4cbd837f34612dfcd82fc4305a898be3209ff7fefc9de7f7f86ca1c63064|
|coherent-privacy-v1/timeline/timeline-withdrawn.json|dfd03aa9681eb54ed94fdb7419f42f551560bd1b46671e8ea1b0bd2782fea424|
|composition-v5/tests.jsonl|d7ff365c526abb46ed8d9ba5755b97652b1b23098779150e13bd812a50a1ddea|
|coherent-experience-v1/events.jsonl|fb775cad55d64271593243e3559593fc6bb73a78eb49fecb06222f39fb4806eb|
|coherent-experience-v1/timeline/timeline-experience.json|99aef0beb0f9779327becada57909d1256ea4bb3aff60dfa487658b671e264de|
|interactive-v5/tests.jsonl|9899054feab7b7bf98057ae718489ae7feb3b511b1fc8a28193971df7a9a573c|
|interactive-v5/actual-interactive-index.json|48c7a2d1ab02743675aecf5f5720f6b5380a77970311031997348e4c815886e7|
|disk-v1/tests.jsonl|ce26512472b82312376476bf2ec77f5f4dcfd808c4c795fa60f85af26479c057|
|payload-backup-v1/evidence.sha256|afd956a22e140e23b1e17588dae0d20a4772446d8ec5861ddc4b05d5c04a0910|

The backup's three phases had already passed when host-side checksum collection
could not read the capsule-owned0600 anchor. Collection alone was repaired in a
read-only offline capsule; permissions, phases and measurements were unchanged.
The retained completion receipt explicitly names that collection repair.
Doctor-v1 also retains a failed owner-policy test: the fixture omitted mandatory
DeliveryID. Correcting that fixture did not weaken owner verification; its actual
PG rerun passed in composition-v1 and remains separate from consumer quality.

## Still required before a supported-mode activation

The current combined subset includes080 terminal-attempt and trusted tool-protocol
integration; later material changes still need their relevant fresh checks.
Direct typed lessons must not bypass applicability via consolidated,
summary, skill or temporal-fact representations. Current exact-tool/environment
and negative feedback guards must apply through the whole original graph.

Native88v4 completed12/12 originals and retained45/45 known full native usage,
but its [independent semantic review](issue-88-native-v4-independent-review.md)
found three failed answers. It is NO_GO and does not qualify temporal reads.
The prospective facts clarification and mandatory expiry fences do not alter
that old result. Temporal-context-v1 also retains a rejected test fixture that
used the profile-only owner proposal endpoint for an episode; v2 uses the real
transactional neutral-candidate contract without changing its validation.

The complete [native88v5 cohort](issue-88-native-v5.md) then completed12/12 with
36 known native entries,248587 full tokens and no product NULL usage. Its
[independent full review](issue-88-native-v5-independent-review.md) passed11
originals and rejected one unsupported inference from a filtered result to
nonacceptance of an old source. Actual model-visible context corroborates the
other disputed timestamp. V5 remains NO_GO; the separate general revision3
clarification changes no old answers, charges, input schema or source limits.

The subsequent [v6 whole-window review](issue-88-native-v6-independent-review.md)
retains7 completed/1 failed/4 unexecuted originals. Its source-currentness timeout
is localized to the original run-source SQL, with23 known native totals and1
unknown. Seven target conclusions are supported, with separate timestamp/expiry
terminology limits; no complete quality or availability gate passed. Exact main
auth handback and paid-discovery200 passed without changing the application.

The [first89 native cohort](issue-89-native-v1.md) is also NO_GO: one background
grouping completed, the first baseline foreground returned capability_mismatch,
and seven slots remained unexecuted. Both native entries have known full usage
(15625 tokens); no Broker tool dispatched. A general trusted single-proposal
instruction and content-free denial reason passed98 controlled checks with no
failures/skips, race and vet. Model compliance still requires a new full cohort.

The complete [native89v3 independent review](issue-89-native-v3-independent-review.md)
now passes the bounded owner-selected screen: all9 slots,23 original units and27
actual source pages. Including preparation, full tokens are108855 versus176055,
while wall time and request bytes increase. No broad performance claim or
production qualification follows. The subsequent complete88 V7 screen passes
all12 primary targets, but its independent full review is NO_GO at10/12 because
two extra verification-expiry assertions confuse applicability with verification.
Its provenance/citation and36 known full-usage observations pass separately.
The general facts4 declaration and10 focused actual-PG/Broker/race/vet events
preserve these dimensions; they do not replace a fresh full native screen.
The native90 experience screen with independent effect/full-cost review also
remains required.
Issue87's earlier incomplete window and
the later complete v5 diagnostic rejection remain retained,
with adaptive retrieval code-unqualified and disabled. Closed native windows are
not selectively rerun or reported as passes. A larger finite capacity is a new
prospective complete cohort, never a refund or rewrite of previous spending.

Issue91's [complete mixed workload](issue-91-combined-capacity.md) and separate
private-PG buffer-restart slice pass on the finite2+2 CPU deployment-build profile.
Warm/mixed p95 is1305.977/1413.040 ms;97 mixed requests complete and3 retain explicit
typed privacy denials. Earlier race windows keep their latency/availability misses.
Actual local query/index IPC and admitted background preemption/recovery are
measured; cloud providers are controlled, so native cloud p95 remains unmeasured.
The final92 pilot must exercise the ordered
preference→owner decision→new conversation→current exception→long task/restart→
verified episode/consolidation→late withdrawal/restore path on one accepted
composition, including explicit model/database/budget/disk failure behavior.
The16-checkpoint controlled privacy extension proves late withdrawal on the
earlier timeline. The separate17-checkpoint extension now proves an independently
verified episode on that earlier timeline. Consolidation followed by withdrawal,
actual custom restore and ENOSPC now consume the episode's same state in the
complete controlled23-checkpoint v8 scenario. Its actual receipt and exact
limits are in [the coherent lifecycle report](issue-92-coherent-pilot.md).
The subsequent complete v9 adds an actual dispatched uncertain effect before
consolidation and checks its unchanged UNKNOWN/no-final/no-replay state through
deletion and restore. Its full24-checkpoint receipt is separate from v8's
owner-recorded compaction state. Native quality and current combined composition
remain separate acceptance requirements.

The subsequent owner-context audit restricts all pre-v10 coherent foreground
receipts to their measured mechanics: the setup actor reached the worker and
could bypass ordinary source-grant/finalizing checks. Original artifacts remain
immutable. Corrected neutral-worker v10 passed the first19 sequential checkpoints,
then reproduced the learned-source final persistence denial after an actual
consolidated Broker read. It did not reach dump/restore and is a failed complete
window. The current worker publication fix and fresh full scenario remain gates;
the earlier physical deletion/restore/ENOSPC measurements are not that gate.

Reconciliation foundation081 is now composed with082. The empty-history
upgrade fixtures roll back082 before081/080/079/077, then apply them forward;
populated provenance is never downgraded. `reconciled-working-upgrades-v1`,
source `29bbcaf7`, passed11 test/subtest events,0 failures/skips, race and vet on
actual lab PostgreSQL. Both primary-input/tool upgrade controls and the strict
working-erasure/replay control passed. Its events SHA256 is
`3aff50f23751d2e880b711e4e5ec2407e7bb5c4d8f2ffea183cd5e82afafddc5`
and evidence-list SHA256 is
`e3a6744f228ee9abb686a254412825166747321d96ec9db2b0111a3887a720f3`.
The witness-only privacy cascade and ordinary captured-lesson publication
prerequisites remain under repair; this focused upgrade receipt is not their
acceptance or the fresh complete composition.

Historical release/bootstrap failures reproduced on unchanged baseline remain
separate; these new component receipts neither waive nor repair those gates.

The subsequent current candidate integrates the captured-publication and
witness-privacy fixes. Their separate actual-PG/race/vet receipts are recorded in
[the experience report](issue-90-episodic-experience.md); they do not replace the
failed neutral-worker v10 window. The next complete controlled lifecycle must
pass all24 checkpoints, retain its actual UNKNOWN action identity, and include
the four final-publication/lease controls. The combined driver now also requires
these controls, witness-only erasure, the native-capture immutability regression
and the v6 carry/neutral-worker controls. Fresh combined and lifecycle receipts
remain required after aligning the experience helper with the production active
source wrapper.

The [complete adaptive v5 review](issue-87-native-v5-independent-review.md) is now
closed:95 completed originals,1 failed transport original and0 unexecuted slots.
All96 answers/outcomes were independently reviewed. Adaptive retrieval remains
rejected and disabled:12/32 complete answers versus27/32 for static retrieval,
with31/32 adaptive plans selecting no retrieval. All98 native entries,97 known
full totals and1 unknown remain charged. The separately proven collector alias
defect invalidates all96 original capture hashes without changing the retained
visible inputs or answers. Its fix cannot retroactively repair that cohort.

The first ancestry-facts runner omitted the compiled guarded Broker process;
17events passed and2mandatory integration cases skipped. That window was
rejected. The v2 driver supplied the actual current Broker test binary and
passed all19 events. A model-input failure in interactive-v2/v3/v4 revealed
a confirmed production formatter defect: untitled selected documents were
discarded after successful actual retrieval. The implementation now retains
them under a neutral label; v5 fixes the formatter rather than changing input
titles or the latency gate. All original failed/diagnostic windows remain.

The foreground verification revision `53d68390` raises the default complete
source proof from200ms to2s after the separately retained native88v6 timeout.
This follows the owner's explicit authorization to increase frequently hit
limits. Trusted explicit200ms..2s bounds remain valid; earlier parent deadlines,
Stop, current post-wait source/authority proof and held DATA remain mandatory.
The default nil-store experience adapter inherits the Core default. Background
200ms and ownership750ms checks, provider300s and qualification flags are unchanged.

Fresh `issue90-foreground-bound-20261018/reports/foreground-bound-v1` tested that
source on actual isolated PostgreSQL with race detection and vet:37 test/subtest
events,0 failures/skips. Both default and explicit2s paths held final/tool/batch
DATA through300ms proofs, denied privacy withdrawal/cancellation and late lease
expiry, and stopped an unreleased proof at2s. The existing50ms parent clamp and
usage-drain/currentness controls passed. Evidence-list SHA256:
`83dc388a2f89600bd091e2194006d47e5d7954f5bd6a6219375a9f948d8aefbd`.
This controlled receipt does not repair or qualify any closed native cohort.

Source `4d7e6a68` now starts fresh combined `composition-v6` and full controlled
`coherent-lifecycle-v11` windows. The full lifecycle-v11 passed24 checkpoints and47/0/0/race/vet; its evidence-list
SHA256 is `6a9b91b8fa80c0c7d7b248f5bd3684e753f23f7fecc11fdf82b81e71ec2ea6f9`.
This is a mechanical PASS for all24 checkpoints. Its early shared prefix used
manual adaptive-store construction and `temporal:true`; it did not establish
the final selected static bootstrap configuration. The separate
[effective-composition delta](issue-92-effective-composition.md) replaces those
fixture overrides with actual startup predicates. A fresh full24-checkpoint
receipt under that delta remains pending; the retained v11 receipt is unchanged.
The narrow delta subsequently passed79/0/0/race/vet and its15-checkpoint static
prefix in `issue92-effective-20261018/reports/effective-v2`; it leaves release
qualification false and does not replace that required fresh full24 receipt.
The original combined-v6 window was subsequently rejected for a missing history
fixture; its fresh passing v7 successor is recorded below. The combined
gate additionally requires the new default/compound held-event controls and
consolidation-v3 worker, failed-closure, exact latest-leaf and current-day carry
controls. Namespace consumption remains tested only in its dedicated disposable
PostgreSQL. The new consolidation history retains all73 current-owner native
attempts including the unknown entry; the separate adaptive population is not
rebilled into that carry.
`composition-v6` retained459 passed test/subtest events,0 failed and1 skipped.
It is rejected as a complete composition receipt: the mandatory
`TestTemporalV5FrozenHistoryWitnessesRejectUsageForgery` had no immutable-history
mount. The driver reached no final vet/completion receipt after that skip. No
native calls or auth were involved, and the old stream is not rewritten.

Driver revision `e09638dd` requires an explicit immutable history path plus its
reviewed SHA256, mounts it read-only, records every fixture-file checksum and
checks them again after the suite. Fresh `composition-v7` uses original
`issue88-20261016/reports/native-v5-r5/history`, manifest SHA256
`50bcbf547df876cd92c36571336dd8a558bbf688bec5c81729144146c39b50d4`.
The same original manifest is independently loaded positively through the
consolidation-v3 recursive-history preparation; the four altered-usage cases are
therefore tested against a validated original. The fresh full v7 result passes as recorded below.

To reproduce the current combined driver, set
`BUTLER_COMPOSITION_TEMPORAL_V5_HISTORY` to that operator-reviewed history directory
and `BUTLER_COMPOSITION_TEMPORAL_V5_HISTORY_SHA256` to its exact manifest hash before
calling `memory-composition-lab.sh ROOT FRESH_LABEL`. This fixture contains original
bounded ledger/history JSON, with no cloud-auth mount or inference permission.
Current `composition-v7`, frozen source `e09638dd`, passes464 test/subtest events,
0 failures/skips, race detection and targeted vet. All mandatory parents passed,
including the four original immutable-history usage forgeries. Both source and
fixture checksums remained unchanged. No auth or native inference was used.
The earlier rejected v6 stream SHA256 remains
`b4d4f7dd92254d57fc8bb850c4420a61069f67148288b13cacb6c86c22c0010b`.

| Current v7 artifact | SHA256 |
| --- | --- |
|tests.jsonl|af5a9ba51a3a7fce46392c89482f972563ae382375ff1bc92a003cf06be666a5|
|history-fixture.sha256|a167236f1a7c2bd759b9abe925e697c5792a5c7dd274dc86f513f8395a27c66b|
|evidence.sha256|1ad80f9afc181a6f5e2c23653a8d7e22b093ab69ba78c8ce010bb660691a928a|

Reports are under `issue92-20261018/reports/composition-v7` on laba. This current
combined receipt and full neutral-worker lifecycle-v11 satisfy their mechanical
boundaries. The bounded mixed-workload profile now has its separate passing
receipt. Temporal/experience native quality and full cost, qualified effective
composition and staged real owner review remain independent gates.
