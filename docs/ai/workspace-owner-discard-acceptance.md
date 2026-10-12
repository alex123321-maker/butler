# Workspace owner discard acceptance

Scope: concrete discard-only recovery for issue17; no deployment, model calls,
account material, source resets or issue closure. Based on merged PR71,90ad32a.

The explicit owner choice is physical erasure of an exact private copy, followed
by monotonic resource reconciliation and permanent copy retirement. Original
effect remains UNKNOWN. No bytes are adopted, write replayed, allowance refunded,
worker lease created or stopped task resumed. Authenticated adapters use
`RecoveryController.Get` / `RequestDiscard`; the installed runtime consumes only
its profile's committed decision queue. Public status contains opaque copy/task
identity, CAS, phase and original-outcome enum, with no file body, root, lease,
fence, source text or credential.

The new migration records an immutable actual executor inode digest on running
dispatch, immutable owner delivery/fingerprint/target and monotonic one-use
erasure phase, plus separately immutable independent receipt. The recovery
guard requires the full terminal owner tree and no live session executor lease,
current original graph/privacy anchor, exact old claimed/unknown lease/version,
natural expiry, actual matching root flock and this copy's physical lane. A
replacement root or missing historical executor binding denies, preserving
unknown state. Commit-before-effect `erasing` permits only synced absence
observation after restart/lost ACK; a present copy needs a distinct authenticated
owner delivery and previous-recovery CAS. Original history prevents rollback.

Actual PG fixtures use pool2, private real files, real private gRPC dispatch,
input-derived natural1.5s resource lease and independent Observer. A fixture
publication trigger fails after the physical pending checkpoint/rename, giving
real rolled-back create and quarantined write with original effect_unknown.
This exercises honest filesystem/PG separation without production clock/hooks.

Initial narrow race/vet PASS snapshot20261012-143808-859, proof
`/srv/storage/butler/agents/workspace-owner-discard/reports/workspace-20261012T073814Z-511118`.
Final narrow source snapshot20261012-145954-589 PASS31 named tests/subtests,
14 top-level tests,0 failures,0 test skips; race and vet passed. The command-only
`apps/tool-workspace` package reports no test files. Proof:
`/srv/storage/butler/agents/workspace-owner-discard/reports/workspace-20261012T075958Z-592418`.
Events SHA256 `06d5bc9a41318781dfa7ba1192237c7cc906038c9652cc6f944d4faf1b413ea5`.

The final tests include an actual Runtime subprocess holding the matching
opened root, refusal while it lives, SIGKILL and kernel release, followed by
explicit owner recovery through a fresh Runtime. They include real pool2 row
barriers followed by source/resource change and refusal, real private Create of
another copy while the first copy lane is held, immutable original dispatch,
root binding, decision and receipt, and fair queue progression past16 deliberately
unusable retained rows without treating those rows as authenticated authority.
The independent Observer refuses a present namespace, symlink, host path and
cancelled observation. Missing receipt/current retirement cannot be replaced by
the terminal phase enum.

Owner-only `ListForTask` discovers an unknown Create's opaque copy ID without
reading private authorization rows in a channel adapter or exposing them to the
model. Its bounded50 metadata observations put unresolved copies before retained
terminal history; only current full proof can enable discard. The profile worker
uses a transient fair scan cursor and two bounded workers, with no persisted
authority in scheduling state. Shared terminal-tree validation excludes exclusive
child/owner mutations while retaining independent per-copy physical concurrency.

The broader `all` race/vet/build suite on pre-discovery snapshot144617-977 passed
at proof `workspace-20261012T074830Z-548342`. The final `all` source proof is
recorded below after current validation.

Run only on laba with a unique disposable root:
`scripts/dev/workspace-lab.sh <isolated-agent-root> recovery`, then `all` for the
affected runtime, source/Catalog, Broker, action authorization and application
consumers. Existing actual Chromium/extension/native-host acceptance is separate
and uses no owner's account/profile. The owning application API/UI and combined
budget/memory/browser/workspace matrix are coordinated separately by root and
the owner-control agent. Do not claim full issue17 from this kernel proof alone.
