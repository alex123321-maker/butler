# Persistent task and bounded delegation ownership

`OwnerOverview` is an authenticated read-only repeatable-read projection of task
authority, initiative, direct children, typed action effects and shared root
budgets. It excludes source/argument bodies and keeps verified completion,
dispatch evidence, conservative reservations and nullable measured usage
distinct. Reads have a two-second bound and explicit child/action truncation.
Actual disposable PostgreSQL tests use `scripts/dev/task-observations-lab.sh`;
the projection does not admit work or charge a model attempt.

The owner snapshot now includes bounded attempt/result metadata, pinned action
source references and current goal/mandate revision/state observations. Delivery
aggregates all chunks; verified obsolete results remain separate from current
completion. Only safe typed references are exported, with explicit omission
counts. `scripts/dev/owner-state-lab.sh` exercises real PG and HTTP lost-ACK replay,
versioned results, safe approval projection and fake Telegram without inference.

Task Store owns durable work, working-state versions and evidence results. Tasks
differ from runs, model finals and delivery. Owner controls apply revisions/stop
across a stabilized cross-session tree; verified completion remains explicit.

Owner controls keep the exact content-free working deletion marker after Catalog
has already cleared memory. Task/working revisions still advance; arbitrary
working content takes the normal clearing/publication fence. The real PG
`TestOwnerControlsAfterSourceDeletionIntegration` covers Delete→Stop/Amend,
stale worker publication denial, fresh amendment source denial and retained
confirmed action evidence. It uses component run/effect fixtures without inference.

The app's existing `TestFullHighDelegationIndependentDecisionsWithoutInferenceIntegration`
also checks a single shared PG/source/budget fixture across actual width1 and
width2 Broker compositions. It independently accepts/rejects width1 via owner
HTTP, checks current width2 reports, then applies Amend/Delete/Stop baselines and
denials while retaining four confirmed effects. Both roots and the fresh amendment
input are durably stopped. Existing scripted observability separately checks owner
Stop after two failed child core entries with no child finals. These scripted
checks prove controls and structured owning reports, not native model quality.

`DelegationManager` adds atomic child/session/contract/input admission on 057.
Its constructor requires shared budgets and every owning Session/Input/Catalog/
policy adapter. No production delegation endpoint, Broker tool or scheduler is
enabled by this component. See the
[contract](../../../../docs/architecture/delegation-contract.md).

Admit/Decide/SetDependencies require a current private parent run or authenticated
owner, plus the mandatory exact proposal/evidence guard. MarkExecuting requires
the child's current fenced run. Advance/Collect use current task/goal/source
authority and never an ended parent lease. Every adapter joins the caller TX.

Admission derives ancestry/IDs/depth, checks narrower descendants, reserves
shared root/mandate/day/ancestor child allowance, then binds exact source grants
before publishing a receipt. Sources/task/contract/input/spend commit together.
Immutable proposal source/version and parent revision provide repeat-safe
identity under the tree barrier; changed content conflicts. Deadline is UTC at
PG microsecond precision; retries cannot extend deadlines or attempts. Handoffs
and decision metadata contain no narrative, parent transcript or credentials.

Defaults allow one queued/executing child; an explicit limit may enable two.
Maximum depth is three, total admitted branches eight, dependencies eight.
Waiting sibling edges may change after current-authority checks; cycles, self/
duplicate/foreign edges are rejected. Dependencies proceed only after guarded
acceptance. Rejected/blocked/cancelled dependencies block dependents. Results
free slots without becoming verified task completion.

Get is a metadata read model, not permission or report content. Collect binds an
actual completed run result to a current structured Catalog report. ParseReport
bounds DATA: accomplished/action/evidence/reservation references, effect states,
eight budget counters and reason enums. Narrative stays in deletable Catalog
data. CheckReportTx compares actual owning records; CheckDecisionTx proves the
objective/owner verdict. Agreeing models cannot bypass it. Decide never marks a
task complete or overwrites another result; conflicting reports remain distinct.

`SaveVerifiedResult` requires a verified channel owner actor matching the task;
anonymous/foreign actors and any worker context deny before a verified row is
written. `Verifier: owner` in result data alone grants no authority.
`AcceptVerified` handles an already collected unverified final: it requires a
separate owner-verified result for the same completed run, current original
report and exact evidence, then generates and accepts a fresh report atomically.
Original result/report IDs remain in decision history. Existing decisions,
different runs, stale/deleted sources, unfinished executions and uncertain
effects cannot be promoted. It does not complete either task or admit work.

`ObserveFailedAttempts` is a separate current-authority read for a blocked
immediate child with failed terminal runs, unchanged contract constraints and no
live worker or unfinished run on any revision. Historical run/action revisions
stay explicit when `SaveRunOutcome` increments the blocked-task revision. Its mandatory
`DelegationAttemptObserver` adapter joins the same transaction to read journal,
all branch reservations and nullable actual usage; it allowlists error enums and
excludes raw provider prose. It cannot create a result/report source, change
state or free execution width. Current parent Catalog dependencies may be pinned
to the parent run. Ordinary execution/collection still requires its original
active-child/deadline guards. Parent context/recovery composition and actual
pool2 tests are described in `docs/ai/delegation-attempt-acceptance.md`.

Lock order: tree barrier → sorted ownership → sessions → current runs → tasks
→ owner/source authority → budgets. Rechecks use DB time after waits. No
ownership.Begin extra connection is used. Owner stop uses the same tree barrier
and captures every committed child session.

Actual disposable PG17/MaxConns2/race tests cover single then bounded parallel
admission, fresh session/working state, restart continuation, dedup/conflicts,
late source-bind rollback, shared spend, depth/tree caps, dependency cycles,
model-verdict denial, stale parent/source, worker lease expiry during authority
wait and Controller stop versus Admit. The
[acceptance audit](../../../../docs/ai/delegation-manager-acceptance.md) records
the exact lab proof and pending product criteria.
The explicit policy fixture checks real source snapshots/anchor and references;
it does not impersonate production transitive child grants or Broker execution.
No inference or external effect occurs in these tests. Production source-only
context without ambient profile/retrieval, Catalog/Broker/endpoints and shared
model comparison remain required before full issue16/33 acceptance.

`CollectGenerated` requires the optional trusted report-creator adapter. It
derives a fresh report from actual owning records and publishes it together with
the collection CAS; ordinary explicit-report collection still compares every
observation. Replay reuses the pinned report. Neither form verifies task success.

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/durable-tasks
ssh laba 'bash /srv/storage/butler/agents/durable-tasks/current/scripts/dev/delegation-lab.sh /srv/storage/butler/agents/durable-tasks'
```
