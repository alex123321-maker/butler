# Resource and workspace adapter acceptance

Component evidence for issue17. The production Broker/runtime path remains
disabled and full issue17/33 closure is not asserted.

Actual laba source snapshot: `20261011-075148-875`, Go1.27.2, disposable
PostgreSQL17/pgvector, two pool connections, actual private filesystem and
initialized deletion anchor. `workspace-lab.sh` ran the whole workspace package
with `-race`: thirteen top-level tests passed, zero failures or skips. Six tests
exercise the new actual resources.Store/WorkerGuard adapter; seven retain the
prior store/path/CAS/authority/JSON checks.

Private proof:
`/srv/storage/butler/agents/durable-tasks/reports/workspace-20261011T005151Z-2641991/tests.jsonl`

SHA256: `08486eb25ba62d1ea3a7d9f706f0f9af19bbd99655fe7f30aec5c05e796c13f5`.
Narrow lab unit/compile suites for workspace and resources also passed.

| Boundary | Actual evidence |
| --- | --- |
| Namespace effect | Real private dirs/sync/metadata; independent fixed-root observer; committed claimed lease consumed, resource version1→2, no invented file hash |
| File effect | Copied actual bytes hashed before first policy check; physical observer verifies confirmed snapshot; file/evidence/lease consumption/resource version2→3 commit together |
| Pre-effect claim | Held lease denied; accidental ClaimTx on the workspace TX denied; no physical namespace created and claim rollback proved |
| Scope/freshness | Tampered body/path, another owner/session/task, task/constraints revision, revoked exact action, terminal run and actual Catalog access revision denied before bytes appear |
| Contention | Two independent lease admissions produce one grant/one conflict; two Store instances sharing a committed claim produce one write/one consumed-proof denial, no overwrite/deadlock |
| Observations | Stale resource version cannot reacquire; stale file snapshot cannot patch after fresh resource admission |
| Unknown result | Namespace/file observer failure after actual physical effect leaves synced pending marker and unchanged claimed/inflight lease; rolled-back metadata/evidence does not imply physical rollback |
| Reconciliation | Lease expiry cannot admit a successor; owner alone or a stop boolean cannot clear it; synchronous executor return plus actual durable run cancellation/fence invalidation and exact independent evidence is required |
| Separate recovery | Resource reconciliation invalidates observations but does not remove workspace markers, rewrite file metadata or unblock a quarantined copy |

The fixture supplies explicitly approved exact action rows and checks real current
Catalog source/anchor snapshots. These action/evidence adapters are test-owned,
not a production Broker authorization, budget reservation or remote executor
stop mechanism. Namespace/file observation uses an operator-fixed private root
and actual bytes. The synchronous fixture executor is known to have returned;
its durable run/fence is also invalidated before accepting stop evidence.
Production owner reconciliation must independently prove actual executor stop
and effect evidence for the exact lease/version, not accept a model assertion.

No new dependency, model tool, deployment route, shell execution, credential
mount or automatic replay was introduced. All proof fields and actual body digest
remain private JSON-excluded kernel metadata. Production exact action/source/
budget callbacks, independently owned effect journal/evidence, resource/browser/
delegation runtime controls and their combined acceptance remain required.
