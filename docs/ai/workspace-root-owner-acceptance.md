# Workspace root lifetime ownership — 2026-10-12

This completes one prerequisite for issue17 recovery after PR70, on a separate
branch from `e805a9c`. The private production Runtime previously allowed another
participating process to open the same artifact root, and Close could close file
handles during active execution or privacy cleanup. The runtime now holds a
nonblocking exclusive Linux `flock` on the actual directory inode, with no shared
lock/PID/temp file. Independent private roots remain concurrent. Other platforms
and filesystems without directory-lock support deny startup with a fixed safe
error. Private mounts, os.Root containment and OS/runtime isolation remain required.

Entry/readiness and the actual Store admission callback compare the held inode
to its current private operator path, including after authority/resource row
waits. Privacy erasure checks the same identity before and after physical work.
Close excludes new entries, drains existing Execute/readiness/privacy passes,
closes their handles and only then releases the root lock. Close is idempotent.
This lock grants no tool, owner, resource or reconciliation authority.

## Startup handle binding correction

Read-only cross-review found a concrete gap beyond the initial green evidence:
the owner could hold inode A while the independently opened Store, Observer and
DeletionRuntime handles were anchored to temporary private directory B. Returning
A to its original name before the final owner check would pass that named-path
check while keeping those B handles open.

Startup now separately compares each actual opened component handle to the
RootOwner's locked inode before returning, alongside the current named-path
check. A deterministic regression uses the same real constructors, opens all
three against temporary B, restores A and proves owner.Check still succeeds
while startup binding denies Store-only, Observer-only, privacy-only and all-B
combinations. Good original handles still pass. There is no production test
hook, custom clock or timing-dependent swap. The test restores real directory
paths and leaves workspace-operation and PG effect state unchanged.

Supplemental final source snapshot: `20261012-030630-685`, isolated root
`/srv/storage/butler/agents/workspace-root-binding`. Both runs used Go1.27.2,
actual disposable PostgreSQL17/pool2, actual private filesystem handles, race
and targeted vet; the all-suite also built the actual static workspace service.

| Runner | Report suffix | Result | tests.jsonl SHA256 |
| --- | --- | --- | --- |
| `workspace-lab.sh <root> ownership` | `workspace-20261011T200649Z-2575205` | 10 passes, 0 failures/skips | `1f5e4607c3e7511fa31e1ac623988b3e2911f47f100df64c15638f0c63852e5e` |
| `workspace-lab.sh <root> all` | `workspace-20261011T200657Z-2575116` | 417 distinct passes, 215 top-level, 11 packages; 0 failures and the same 11 explicit opt-in skips listed below | `9882cd740b539deb65a88300cf71c28a2ed16b8ef4a96ca24899888c16a92355` |

Owned containers/networks were removed; retained proof/PG files were preserved.
This correction used zero model calls and changed no deployment, credential
material, current main checkout or issue status. Owner recovery and the other
remaining gates below are unchanged.

## Initial root lifetime evidence (before handle binding correction)

Root: `/srv/storage/butler/agents/workspace-root-owner`. Final source snapshot:
`20261012-024854-794`; Go1.27.2, disposable PostgreSQL17/pool2, real private files,
real subprocesses, race detector and targeted vet. No model/provider calls,
credential imports, deployments, account changes or issue closures occurred.

| Runner | Report suffix | Result | tests.jsonl SHA256 |
| --- | --- | --- | --- |
| `workspace-lab.sh <root> ownership` | `workspace-20261011T194911Z-2506503` | 5 test names, 0 failures/skips; race and vet | `2d0ea727277f581c06d126f12f5cec786ffe66979a5a01a4ce4525c659921435` |
| `workspace-lab.sh <root> all` | `workspace-20261011T194920Z-2506417` | 412 distinct passes, 214 top-level, 11 packages; 0 failures, 11 explicit skips; race, vet and static actual service build | `49b8b85e1ae73f357996dddf5f7b30120ccf80009d29c3eed3b4b440763c6156` |

The subprocess regression starts an actual owning process, rejects a competing
process, retains concurrent ownership of an independent root, SIGKILLs the first
owner and admits a replacement only after that process actually dies. No root
files appear. Other tests reject shared permissions, symlink roots and replaced
paths while retaining ownership of the renamed original inode.

The actual PG regression observes Execute waiting on its owning input row through
the already borrowed blocker connection. Close remains pending and a replacement
root owner is denied until the actual Execute returns after cancellation. A new
Runtime can then open the root, while the exact prior resource/version/fence,
inflight lease, prepared dispatch and executing effect remain unchanged. Closed
execution/readiness are denied. A second actual PG regression replaces the root
during an authority row wait: both original and replacement directories stay
empty, no workspace/evidence is committed, and the one-use running dispatch and
resource quarantine remain honest.

The all-suite also retains actual Broker/runtime application loopback/Unix and
scheduled source-erasure tests. Owning context and human approval in that existing
fixture remain explicit fixtures. Eleven opt-in skips are retained:

- Broker real browser sequence, actual Chromium page CAS and actual guarded
  native-host Chromium: their separate isolated browser images are required.
- Broker native delegation and ordinary delegation harness entries: private
  process-harness opt-ins are required.
- Catalog actual dump/restore: its separate external pg_dump/restore procedure
  is required.
- Tool Workspace harness entry: parent application tests launch the compiled
  harness separately; the unselected harness entry itself is skipped.
- Orchestrator approved-credential browser E2E: its separate private authority
  and real Chromium environment is required.
- Orchestrator guarded-tab `broker_process` and memory-candidate `broker_process`
  / `lost_ack`: their separately compiled private Broker process is required.

The unsupported-platform branch also compiled for `GOOS=windows`, with networking
disabled on laba. Its test binary SHA256 is
`813d7960ed4d01d7ade0ab6c248671ce4f0045a00f8b502a56bb62ddcea6c8d7`.
This is compilation, not execution of Windows tests.

## Retained failures and remaining gates

The first narrow fixture run `workspace-20261011T194133Z-2472086` failed because
its new test roots were not explicitly provisioned0700. The production guard
correctly denied them; only test provisioning was corrected. Event SHA256:
`44ba2547f3cc86e2bc65e4c23e3539e86766e090197be2bfa73bd115656291de`.

The first broad run `workspace-20261011T194618Z-2493528` retained an unrelated
`CREATE SCHEMA` collision in the shared memorytest fixture. Its schema suffix
was raw Go wall-clock time, shared by concurrent package processes. The fixture
now uses128-bit `crypto/rand` identity with a fixed entropy-failure error and
early administrator cleanup. No dependency or production clock policy changed.
The failed events remain SHA256
`502d276107343cff6d56a1611981a4bb37609936205e93d1afd5a337015df540`;
the final all-suite above passed the actual physical Observer regressions.
Owned test containers/networks were removed; PG/proof data remains retained.

Issue17 stays open. SIGKILL release of a process lock does not prove effect
absence, reconcile an unknown resource, erase quarantined artifacts or authorize
replay. Separately authenticated exact owner recovery still needs independently
obtained effect/artifact evidence, actual old-executor quiescence, expiry,
monotonic fence/observation invalidation and retained journal/history. Real lost
filesystem/PG/RPC acknowledgements, task-tree stop and the combined tab/file/
budget/memory scenarios remain required. No owner-abandonment endpoint, discard
receipt, new recovery migration or production rollout is claimed here.
