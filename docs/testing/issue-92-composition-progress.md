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
The current publication/privacy fixes still require a fresh full lifecycle and
combined composition pass; the earlier table rows do not qualify this candidate.

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

New89 and90 screens, a fresh88 semantic screen, independent effect review, complete usage and
declared capacity remain separate. Issue87's earlier incomplete window and
the later complete v5 diagnostic rejection remain retained,
with adaptive retrieval code-unqualified and disabled. Closed native windows are
not selectively rerun or reported as passes. A larger finite capacity is a new
prospective complete cohort, never a refund or rewrite of previous spending.

Issue91 now has two actual controlled foreground/background consumer overlaps
in addition to local query/index overlap. These are individual bounds, not native
p95 or the complete mixed workload. The final92 pilot must exercise the ordered
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
