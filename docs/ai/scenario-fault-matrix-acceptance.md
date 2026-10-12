# Issue33 product fault matrix: owning boundaries and remaining gates

This additive matrix follows main3d2b44b. It does not change the frozen Russian
corpus, independent golden approvals, scoring criteria or historical reports.
Those artifacts remain owned by `internal/acceptance` and their separately
appointed reviewer. A deterministic scenario PASS is not model quality, complete
cost evidence, permission to deploy or full issue33 acceptance.

| Required user outcome / fault | Owning implementation and reproducible evidence | Qualification |
| --- | --- | --- |
| Multi-step browser observes before acting; page loss/change stops stale work | `TestBrowserSessionE2E`, `TestBrokerBrowserSequenceE2E`, `TestProcessFailureInvalidatesAllSessions`; current actual Chrome `manual_input`, `dom_mutation`, `navigation`, `lost_ack_restart`, `two_executors`, `installed_native` in [resource evidence](resource-full-acceptance-20261012.md) | Temporary synthetic profile, real extension/private Broker; same-tab UNKNOWN clearing remains unavailable |
| Restart while awaiting approval; fast duplicate and stale reply | `TestApprovalWorkerSIGKILLRestartIntegration`, `TestDurableApprovalDecisionRacesAndInvalidationIntegration`, exact Telegram decision binding in [existing Full fault proof](fault-scenario-acceptance.md) | Actual Linux process death and PG identities; scripted provider, synthetic recipient |
| Owner “не отправляй”, child Stop and stale lease deny effects | `TestControlAmendStopsOldApprovalAndPreservesNewContext`, `TestOwnerTreeStopRevokesActualGrandchildGrantIntegration`, `TestOwnerControlWinsBeforeRealDispatchClaimIntegration`; current [delegation control evidence](delegation-control-matrix-acceptance.md) and [resource evidence](resource-full-acceptance-20261012.md) | Effects claimed before control stay confirmed/unknown; Stop is not physical undo |
| Before/after effect boundary, UNKNOWN and failed result delivery never replay work | Permanent action journal lost-ACK/before-dispatch checks, atomic-result/input recovery, plus the new actual converter result/delivery tail below | External effects and channel acceptance have separate ledgers; unknown usage/outcome never becomes zero/absent |
| Old rule conflicts with owner correction; new task after failed task; deletion survives restore/reindex | `TestSummaryCannotHideRecentOwnerCorrection`, durable task isolation integration, current source/head CAS, actual `pg_dump`/restore with a new independent deletion anchor and reindex denial | Restored private content is denied before reconciliation; no real persistent deletion anchor is touched |
| File/tab conflict, shared budget, repeated loop and branch growth | Actual workspace/private RPC/root-inode/one-use discard proof, actual Chrome conflicts, PG shared root/day/child quotas, `TestNormalizedLoopPersistsAcrossRestartAndTypedErrorsIntegration`, `TestDelegationTotalBranchCapAndMissingAuthorityIntegration` | [Issue17 scope](resource-full-acceptance-20261012.md) explicitly retains browser UNKNOWN quarantine; complete single/multi cost measurement is pending |
| External instructions/child sources/synthetic secret/forbidden resource | Fixed content derivation, owning selected-child Catalog/source guards, denied secret before IPC, workspace/egress containment, source lifecycle and private credential/Broker checks in [security evidence](security-acceptance.md) | DATA does not gain instruction authority; no working secret or real account is used |
| OOM, mismatched schema, failed release, rollback | Controlled local-runtime pressure/fallback/owned-process cancellation; exact typed installed extension/converter readiness and rollback, author mapping/criteria cannot replace independent checks | Controlled OOM is not hardware/model OOM evidence; protected general self-repair release remains pending |
| Mandate → initiative → delegated missing adapter → isolated developer → independent tests → allowed install → task verified → one notice, also restart/Stop/delivery failure | Current [initiative project proof](initiative-high-acceptance-20261012.md) and [converter automatic worker proof](../architecture/generated-converters-acceptance.md) establish separate real owning paths; the new tail joins converter work to owner verification and existing delivery | **PENDING combined chain.** The converter starts from one explicit owner input, not initiative promotion. Its three parent attempt finals create three notices; owner verification creates no new final-task notice. No mock stitching or inherited PASS fills this gap |

Historical evidence is reusable for the exact invariant and pinned source it
actually tested. It does not assert all tests ran again on this source. The old
`internal/acceptance/matrix.go` contains historical pending descriptions; this
document qualifies newer evidence without silently changing its signed criteria.

## Concrete production fault found by the new owning flow

The unchanged explicit converter worker baseline passed, while the unchanged
automatic055 baseline failed at the first `converters.collect` claim. This
was before any new result/delivery hook. The actual private Broker calls
`PostgresStore.guardDispatch` with RPC context. It correctly locks and checks the
stored grant's current PG session/lease/fence/run/task, but previously called its
source guard without the worker token required by `CheckContinuationRunTx`.
Ordinary explicit owner inputs bypass no check: they simply have no055 input.

The fix gives only that same-transaction source adapter a private read-proof
token from the already locked exact matching lease. Caller token and Actor are
overwritten; nil `ownership.Authority` prevents obtaining a new write fence or
borrowing another connection. The private derivative never escapes to the RPC or
runtime. It does not grant a source, input claim, continuation, lease renewal or
new action. Existing source/current-input/055/task/expiry checks and permanent
dispatch/budget/replay CAS remain mandatory.

`TestClaimRunDataGuardDerivesOnlyCurrentDatabaseOwnershipIntegration` has actual
PG MaxConns2 cases for plain RPC, forged caller metadata, naturally expired lease,
superseded lease, stale task, and current-source refusal. A read proof can use its
existing transaction but fails `ownership.Begin`; caller context remains intact.
Refusal leaves the authorized grant/allowed effect and zero dispatch rows. Valid
claim commits once; repetition cannot create another permanent dispatch.
`TestClaimGrantExpiryDuringPrivateSourceReadProofRollsBackIntegration` separately
retains a350ms exact approval/grant. The valid same-TX verifier enters while the
grant is live, waits on actual PG wall clock past its expiry, and the final
fresh-clock dispatch CAS rolls back grant/effect, zero dispatch/reservation rows
and all new budget scopes. Current read proof cannot extend grant permission.

## Actual converter result/delivery tail

`TestScenarioFaultConverterResultDeliveryContainerE2E` reuses the ordinary
automatic converter fixture with observational test-only hooks. Its existing
unhooked explicit/automatic tests retain their behavior. It starts the production
Broker process and the production Go converter binary on a private Unix socket in
a separate UID10001 read-only, network-none Docker container. The developer gets
exactly three reviewed tools, selected Need/contract DATA and no original document.
Fixed independent checker/trial/activation and original conversion still run
through the actual owning services and permanent journal. The deterministic
provider proposes tools and consumes real results; it has no PG/network API.

After the ordinary workers finish and drain, the independent checker compares
the actual original CSV and exact output JSON/hash. A model final, foreign owner,
stale task version and missing criterion cannot complete the task. The current
authenticated owner saves a separate verified result and completes the exact
task through Task Store; a reconstructed store observes it. All four immutable
model finals remain unverified. Delivery then has only Store and a bounded
loopback HTTP channel simulator, with no provider/tool capability:

- `restart_before_send`: a fresh dispatcher delivers the three frozen parent
  attempt notices once and preserves their ACKs.
- `explicit_absence`: recipient rejects the first request before acceptance;
  the actual bounded retry delay expires naturally, and a fresh dispatcher
  retries that same stored content. Four requests produce three acceptances.
- `ack_commit_failure`: earlier two notices ACK normally. A PG trigger fails
  the final ACK write **after** its actual HTTP recipient acceptance. Restart
  cannot resend during the claim. The production30s sending claim expires
  naturally; a reconstructed dispatcher quarantines it as `delivery_unknown`,
  with no invented ACK or repeated send. Earlier ACKs remain intact.

Content-free frozen projections check exact effects, source hashes/revisions,
immutable final hashes/verdicts, completed task/criteria revision,055 identities,
shared budget used/limits/deadlines and reservations across delivery. The fourteen
scripted model admissions, one developer start and ten confirmed tool actions
cannot be reset or repeated. This is a converter tail and controlled channel
fault proof, not actual Telegram delivery, model planning, recipient effect
verification, a one-notice product workflow or general self-repair acceptance.

## Reproduction and report discipline

Run only on SSH laba from a clean committed isolated worktree:

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/scenario-fault-matrix
ssh -o BatchMode=yes laba 'bash /srv/storage/butler/agents/scenario-fault-matrix/current/scripts/dev/scenario-fault-matrix-lab.sh /srv/storage/butler/agents/scenario-fault-matrix authority <exact-commit>'
ssh -o BatchMode=yes laba 'bash /srv/storage/butler/agents/scenario-fault-matrix/current/scripts/dev/scenario-fault-matrix-lab.sh /srv/storage/butler/agents/scenario-fault-matrix regression <exact-commit>'
```

`authority-all` runs the entire affected actionauth suite with actual PG and no
test skips. `tail` is the narrower new three-branch flow; `baseline` selects unchanged
explicit/automatic workers. `regression` additionally requires both baselines
and nine named PG055 continuation tests. Missing, renamed, repeated, skipped or
failed required tests deny the report. Go1.27.2 tests use race detection and vet,
with a300-second outer test-process timeout to cover the separate flows and
natural30-second delivery claim expiry. Product budgets, source/lease deadlines,
required scenario assertions and protected criteria are unchanged.
offline module/build network and repository caches. Actual PG17 and all runtime
resources belong only to this invocation. The converter binary/profile, all source
and migration/data bytes, hardware/CPU/container identities, binaries and JSON
event hashes are retained. The Docker runtime uses the pinned Alpine base and
current compiled production binary; this is not a new production image release.
Raw events, synthetic token, PG files and service logs stay private below the
agent root. No real auth/import/provider/weights, model requests or human messages
are used. Historical native45/20 known/25 NULL pairs and74698/2210 known subtotal
remain sealed; this fixture adds zero native calls.

## Retained diagnosis and current status

The initial runner failed binary-directory access before executing a test;
only its dedicated binary directory became755 and executables555, while private
artifacts and caches remain closed. Proof091240Z-836635 is retained unchanged.
The next three-branch run failed before its hooks at collect; proof
091416Z-842874 is retained. The unchanged baseline then passed explicit9.86s
and failed automatic7.41s, proof091634Z-850994, events SHA256
`3bd3155f508ddbaca7f13aefb069008c6a94cea634cfa30768c1fa7aae444086`.
The new narrow PG regression failed on old code in plain/forged/current-source
cases, proof092255Z-875908, events
`4a3c7d3bbe9e26b7e30f2a0c28d9baa6ca000b8fdd7d8abf9fcc7b2b2c7657f5`.
These proofs are below `/srv/storage/butler/agents/scenario-fault-matrix/reports`;
none is relabelled PASS or used as model-quality evidence.

The fixed narrow PG regression passed all six cases plus its top-level test,
race/vet, zero failures/skips/native calls, proof092401Z-880076, events
`b2b121f0d094a1b0293bff6c177268ace9fb7f4bcf14c14b3202c709660c6f08`.
The subsequent narrow093125Z-906620 also passed the post-verifier grant-expiry
case. The owning converter/055 regression on snapshot `20261012-162647-424`
passed30 named tests including subtests/12 top-level tests, zero failures/skips,
race/vet and compiled production Broker/converter in233.247s. Its unchanged
automatic baseline and all three new tail branches passed. Proof
092654Z-890787, events
`f84fcccfde4ebbb221148e12aa51eb3df9e36d1812cc80c5d14017c12e2b3f0c`,
complete source manifest
`9867487dc6bce4683459646954754b86134b535ba3a8b98a7e2f580ff8db03c9`.
These diagnosis/implementation snapshots include work in progress; their exact
byte manifests are retained and are not represented as clean main releases.
Final committed-source affected actionauth proof is recorded below.
Issue33 remains open until the combined required chain and separately pinned
observation-only Russian model/holdout/single-versus-multi measurements satisfy
their actual independent controls. Existing unknown usage precludes a complete
resource/cost claim by substituting zeros.
