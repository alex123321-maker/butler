# Delegation manager component acceptance

This is component evidence for issue16, not a production release or full issue
closure. No model or external tool effect was executed by these fixtures.

Validated on laba from source snapshot `20261011-073844-801`, with Go1.27.2,
PostgreSQL17/pgvector, private initialized deletion anchor and a two-connection
pool. `scripts/dev/delegation-lab.sh` created its own bounded database/container
and internal network, ran `-race`, and removed only those containers/network.
It did not access shared Compose/main PostgreSQL, model credentials or inference.

Private proof: `/srv/storage/butler/agents/durable-tasks/reports/delegation-20261011T003851Z-2586845`.
`tests.jsonl` SHA256:
`9ffb05880a7776fe13587a119b8fb79dda44bb130c35fc751e52c777552ddd90`.
Six actual PostgreSQL manager tests and the report-shape unit test passed with no
selected-test skip or failure.

| Boundary | Component evidence | Full product requirement |
| --- | --- | --- |
| Independent child | Fresh Session row/working state; opaque contract-source input, immutable deadline/attempts; replay/conflict checks | Explicit production preparation must exclude ambient profile/retrieval and parent context |
| Shared bounded execution | One queued/executing child; explicit two concurrent admissions; actual root child reservations; depth3/tree/dependency caps | Run both executor modes through the same actual model service and compare quality/resources |
| Crash continuation | New manager instance queues durable waiting child without a parent worker token | Owning scheduler/worker lifecycle and actual parent process kill/restart |
| Atomic admission | Late source-bind denial rolls back task/session/contract/source fixture/input/shared reservation | Real current transitive Catalog grant adapter and Broker exact authority |
| Dependencies/conflicts | Sibling cycle rejected, unverified result cannot be accepted; rejected dependency remains blocked | Real conflicting executor reports and independent owning evidence readers |
| Results | Actual task-result/terminal-run identity and strict versioned Catalog report references; acceptance preserves active task status | Actual model/actions/artifacts/budget observations checked by production guard |
| Authority | Current Catalog hash/access/anchor, stale parent rejection, anonymous/nil guard denial, DB-time worker lease expiry recheck | Current goal/mandate/source policy under integrated admission/dispatch/finalization |
| Stop | Actual Controller stop versus concurrent admission, every committed cross-session descendant cancelled, no restart admission | Integrated provider interruption and all delegation/tool/runtime routes |
| Workspaces/resources | Separate component evidence only | Combined finite-resource lease and isolated workspace/Broker flow |

The fixture guard reads real current Catalog snapshots and the real deletion
anchor. It does not grant production transitive access. Its owner-verified result
fixture saves actual durable result rows after a synthetic run terminal transition;
this proves storage/manager contracts, not external success. Report parsing checks
bounded DATA shape only. Mandatory production guards must compare every report
action/evidence/reservation reference with its actual owning record and reject
model consensus as verification.

Manager construction needs owning Session/Input adapters, configured shared
budgets and all five policy/source/evidence callbacks. This component contains no
production wiring or model-selectable delegation tool. Issue16 and issue33 remain
pending until the integrated product and model experiment criteria pass.
