# Native-chain prototype: offline evidence and unresolved failure

The prototype's targeted offline checks pass, but full process acceptance is
failed and native execution remains closed. This is not issue33 acceptance.
No native call, credential import, physical model load or human message was made
by these runs. The prospective480 baseline is not established after the failed
V2 experiment; the compiled prerequisite hash remains empty.

The exact code and report hashes are recorded in
[the evidence file](../../deploy/acceptance/scenario-release-chain-high-offline-20261012.json).

| Source | Actual scope | Result |
| --- | --- | --- |
|4d643d7|PG/current-proof/shared-cap/terminal-cost tests and compiled native prototype|16 named passes,6 top levels,0 failures/skips; race/vet|
|4d643d7|Owning PG completion/outbox and10 current-entry denial cases|20 named passes,2 top levels,0 failures/skips; race/vet|
|4d643d7|Actual Broker/converter processes, scripted planner, loopback Telegram|FAILED: explicit_absence and enclosing chain;7 other named passes|
|b224652|Same targeted checks plus adversarial structural-diagnostic projection|17 named passes,7 top levels,0 failures/skips; race/vet|
|b224652|Once narrow actual-process explicit_absence diagnosis|2 named passes,1 top level,0 failures/skips; race/vet; old failure not reproduced|

The full4d failure reached the unchanged25-second automatic-state fence with
`activated_catalog:queued:running:tool_running` and
`candidate_return:queued:completed:completed`. Its exported events lack the exact
tool/dispatch/deadline snapshot, and normal fixture cleanup removes its SQL
schema. The narrow diagnostic did not reproduce it. Host concurrency, a
premature fence, a lock or a runtime delay remain hypotheses; none is established.
The narrow pass does not replace or relabel the full failed report. Model20s,
tool30s, the25s fixture fence, owner criteria and production decisions are
unchanged.

The added failure projection reads at most12 current root/child Runs and emits
finite public tool names, owning state enums, effect/dispatch state, live-lease
and reservation/deadline flags, elapsed tool time and configured policy bounds.
It excludes arguments, results, private error text, source bodies, provider
sessions and session/lease credentials. Reservation-derived policy deadlines are
labelled upper bounds, since a real authority/resource deadline can be stricter.
Its real-PG negative test seeds private payloads and an unlisted name and checks
that none become exported content. This projection never grants authority or
changes an acceptance verdict.

One preparation invocation accidentally used an incorrect declared release
argument. Its original report143126Z-1940232 is retained and excluded. The
correct b224 preparation143223Z-1943859 supplies the recorded current proof.

The standalone production High generated-total guard was separately tested on
3a69f05 with45 named passes/7 top levels,0 failures/skips, race/vet. It preserves
visible measured pairs and conservative reservations, and bounds High generation
using the complete total. That proof does not establish native model quality.

Future execution requires an actual independent quality/comparison PASS and
current immutable history, an independently frozen prerequisite and protocol,
and a new current combined preparation. The native prototype's real-model
restart/sent, accepted-lost-ACK UNKNOWN and owner-Stop cases have not run. No
future scope, deadline or historical failed report was reopened by this work.
