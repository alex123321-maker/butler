# Workspace original-source and erasure acceptance — issue17

Migration066 and `internal/workspaceexec.Provenance` supply current source policy
and cumulative immutable provenance for prepared code copies. Exact approved
tool arguments, shared budgets and production Broker/runtime composition remain
separate. The tests' action admission is explicitly a fixture; Catalog ownership,
source/task policy, original graph, actual resource claims, workspace storage,
physical Observer, deletion anchor and fixed-root erasure are real components.

Private source context is prepared before effect/budget/resource/workspace locks
from actual run graph roots and prior copy history. Ancestor pins remain privacy
dependencies and cannot become selectable child context. Repeat checks use the
original set, actual current worker/run/task/constraints and a fresh DB clock.
Inherited owner/job proof is rejected/stripped, never borrowed as workspace
authority. After physical confirmation the same transaction publishes empty-body
task DATA retaining all original parents, immutable physical-observation digest
and lease identity. It accepts no facts or policy and copies no source/code body.

Actual acceptance cases establish:

- A fresh owned run with no old model context reads an existing copy only after
  independently checking/pinning its original graph. Private prepared metadata
  serializes to an empty object; scope remains the actual owning task.
- Source delete/revoke blocks the copy and queues exact physical erasure on the
  deletion transaction. A previously successful read is then denied. Actual files
  disappear under a fixed private root; the completion mark is durable/idempotent.
- Failed confirmation after physical rename publishes no new file metadata or
  derived provenance, retains actual changed bytes and blocks reuse.
- Owner/job shortcuts, foreign scope, stale task/constraints and empty contexts
  deny. An actual source-row waiter observed through pg_blocking_pids cannot extend
  ownership past its PostgreSQL expiry.
- UPDATE/DELETE cannot overwrite lineage. Failed erasure keeps the job pending;
  actual erasure with a lost acknowledgement also stays pending, and a restarted
  fixed-root runtime syncs the absent copy before completing cleanup. Cleanup
  cannot reopen the workspace or erase a different copy/link target.

Final source snapshot: `20261011-191340-793`. Public events under
`/srv/storage/butler/agents/workspace-provenance`:
`reports/workspace-20261011T121346Z-973273/tests.jsonl`, SHA256
`53d5fa323aea6623c090413d74aa26141683aa95df89d04d0c3f369450232fce`.
107 distinct test/subtest names (50 top-level) pass in three packages, with zero
failures. The opt-in external PostgreSQL dump/restore procedure is the one skip;
this report does not claim to have repeated that procedure.
All checks run on isolated laba, disposable PostgreSQL17,
real private files, pool2, Go1.27.2, CPU runtime runc, race detector and targeted
vet. No inference, production flag or main deployment change occurred.

Retained earlier successful expanded proof: source snapshot20261011-190719-408,
`reports/workspace-20261011T120725Z-952806/tests.jsonl` under
`/srv/storage/butler/agents/workspace-provenance`, SHA256
`bfbd86f249f16f74748e5c49269707d8091199eac3466e34682fd938ed7668b4`.
106 names passed in three packages, with the opt-in external dump/restore test
skipped. A later expanded attempt at20261011T120932Z-959837 passed the new lost
erasure acknowledgement case but failed a separate workspace fixture during
concurrent pgvector installation (pg_extension_name_index). It is retained with
SHA256 `0db5a466d163d18e509f99ba42ceae43497656f50ca543fa9ccfb57638be61d1`.
The harness now installs pgvector once in its disposable database before running
independent package schemas. No production policy was weakened to fix that race.

This is source/privacy composition, not complete issue17 acceptance. Production
exact Broker/shared budget/ticket/immutable evidence routing, workspace runtime
bootstrap/cleanup scheduling and owner reconciliation remain pending. Native
parallel delegated semantic/cost acceptance remains separately blocked by the
sealed failed provider windows. Do not enable workspace model access or close
issue17 based solely on these component tests.
