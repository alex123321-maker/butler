# Failed delegation attempt observation — issue16

This implements the separate historical observation bridge. Issue16 remains
open: a successful native delegated research run, independent conflicting
reports and the complete single-versus-bounded-parallel experiment remain
pending. The shared deployment still has delegation disabled.

The owning manager reads a blocked immediate child under current parent authority
with no live child lease/attempt. The PG projection reads actual terminal runs,
permanent action/effect states, all conservative branch reservations and nullable
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
- stopped/revised parent, revised/cancelled child and live child worker/attempt;
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
