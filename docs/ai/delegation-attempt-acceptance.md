# Failed delegation attempt observation — issue16

This implements the separate historical observation bridge. Issue16 remains
open: a successful native delegated research run, independent conflicting
reports and the complete single-versus-bounded-parallel experiment remain
pending. The shared deployment still has delegation disabled.

The owning manager reads a blocked immediate child under current parent authority
with no live child lease/attempt. The PG projection reads actual terminal runs
across current/earlier revisions, permanent action/effect states with actual run
revisions, all conservative branch reservations and nullable
actual model usage across task revisions, with each actual revision explicit.
Changing the task revision cannot hide earlier measured or unknown spending.
Known usage7/3 and a separate unknown pair in tests are
explicit synthetic fixture observations, not Gemini measurements. Raw error
prose is excluded before leaving the database; only fixed enum subtypes survive.

The parent context adapter derives candidates from its actual live run, releases
its initial selection transaction and then calls the tree-locking manager. This
avoids a nested connection on a MaxConns2 pool. Preparation/recovery supplies
private SubagentOutput DATA with current source pins; checkpoint bodies become
reload markers. No new completion, decision, refund, slot release, automatic
parent wakeup or generic contradiction resolution is introduced.

Validation uses disposable PG17, a private internal network, two pooled
connections, race-enabled Go tests and vet. Component and context checks cover:

- actual known/unknown usage, permanent unknown effects and branch spending across revisions;
- current owner/parent worker identity, mixed/foreign identity denial;
- stopped/revised parent, changed child constraints/cancelled child and live child worker/attempt;
- corrected/deleted/secret sources and source guard denial after observation;
- expired/ended parent ownership, bounded16-run history with explicit truncation;
- fresh recovery after journal changes and denial when the current reader is absent.

The final pipeline proof is run through the ordinary durable ingress/run/lease,
shared budget and final stores. The provider outcome is scripted and therefore
cannot prove generative quality. A deletion after report preparation must deny
provider entry, and reading the report must leave the child blocked.

Final validation passed on immutable snapshot `20261011-141030-569`:
`/srv/storage/butler/agents/delegation-attempts/reports/delegation-attempts-20261011T071039Z-4185934`.
The `tests.jsonl` SHA256 is
`7a1b42db61264022abbfdb69474b4efd511074b23a7f3620a6c5d8e99fece3ac`.
All four packages passed race-enabled checks, followed by affected `go vet`.
There were264 passed tests/subtests and5 conditional skips: the separate actual
Broker/container converter fixtures and two older service integrations were not
enabled by this isolated PG-only runner. Those skips are not acceptance. The new
PG observation, pool2 context and ordinary parent-pipeline cases all executed.
This validation added zero actual model requests and changed no main deployment.

A subsequent review found that ordinary `SaveRunOutcome("failed")` advances the
blocked task's revision, while its terminal run and action journal retain the
original revision. The original fixture manually set blocked status and missed
this real lifecycle. The corrected fixture invokes `SaveRunOutcome` and requires
the revision increment. Historical attempt/action projection now preserves their
actual revisions, and the ordinary parent-provider boundary checks current2
versus failed1 explicitly. Changed child constraints still deny. Parent constraints
revision2 versus the child's admission revision1 is separately verified.

The lifecycle regression passed the four-package/race/vet runner on snapshot
`20261011-142541-974`, proof
`/srv/storage/butler/agents/delegation-attempts/reports/delegation-attempts-20261011T072601Z-39972`,
tests SHA256 `1b00bc7161be0f8fad2d3c334c7d396e3b4fad27132c796950de6f23f70b8b11`.
The final guard changes (unfinished older run denial and independent parent/child
constraints) passed the narrow actual PG/context/pipeline race suite and affected
vet on snapshot `20261011-143149-090`, proof
`/srv/storage/butler/agents/delegation-attempts/reports/delegation-attempts-20261011T073152Z-60727`,
tests SHA256 `c13a459c4c9d13b432508b0061e85c1e7513b8236c8d0b55404ff0930f7cdca0`.
Both checks made zero actual model requests. The intervening parent-constraints
fixture initially retained its old capability snapshot and denied admission;
that failed run remains recorded at `delegation-attempts-20261011T073025Z-55807`.
The fixture now updates its explicit PG capability snapshot before admission;
production policy was not weakened to make that test pass.

Earlier failed fixture runs are retained in the lab reports directory. They
recorded a wrong `runs.error_code` column (actual column: `error_type`), an invalid
fixture deletion action and a missing fixture event type. Each was corrected;
no native/cloud inference occurred. The deleted-source pipeline case now also
requires proof that the actual observation boundary was reached, preventing an
unrelated earlier error from satisfying its denial assertion.

Run from the worktree:

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/delegation-attempts
ssh laba 'bash /srv/storage/butler/agents/delegation-attempts/current/scripts/dev/delegation-attempts-lab.sh /srv/storage/butler/agents/delegation-attempts'
```

Pass `narrow` as the second runner argument for just the failed-attempt
PG/context/pipeline race checks plus affected vet. The default remains full.
