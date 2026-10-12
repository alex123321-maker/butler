# Resource arbitration and recovery evidence, issue17

This matrix concerns the supported self-hosted Linux runtime and synthetic
browser/project resources. It distinguishes actual owning execution from seeded
presentation, provider generation, human delivery and deployment activation.
The eight safety/arbitration criteria pass within this declared scope. The
owner-request supersession regression and integrated owner API checks passed on
the final kernel; same-tab UNKNOWN unblocking remains deliberately unavailable.

| Criterion | Owning implementation and current evidence |
| --- | --- |
| 1. One executor per owner tab, manual version advance | Private single-tab ticket, PG exclusive resource lane and production extension journal; actual Chrome `two_executors`, `manual_input`, `manual_new_observation` |
| 2. No stale browser plan | Real observation version/hash, current selection, origin/host and DOM checks; actual Chrome DOM mutation, navigation, foreign origin/host and manual intervention deny the old click |
| 3. Separate copies and file conflict | Actual private workspace RPC/files, isolated opaque copies, metadata/generation CAS, independent observation and per-copy lanes; current workspace regression covers different copies and conflicting file publication |
| 4. Finite leases, ordering and defined recovery | Sorted all-or-none sets, at most16 resources and30s leases, no renewal and fresh proof after lock waits. Expired unclaimed acquisition can recover; claimed/UNKNOWN effects remain quarantined. Workspace recovery uses an explicit owner discard, a drained inode-bound executor, a one-use erase phase and independent synced absence; stale queued owner intent has a separate regression gate |
| 5. Expired owner cannot write | Actual authority/source/resource proof after waits, old root/executor binding and real Runtime SIGKILL; stale old leases/worker/source/CAS deny before mutation or erase |
| 6. No workspace transfer of rights/secrets | Exact profile/root/copy/source/owner binding; private process signing/control material stays outside copy/model data; wrong root, missing historical binding, caller path/lease/receipt and foreign owner deny |
| 7. Atomic shared task budgets | Actual PG `TestChildAndToolReservationsShareAtomicParentLimitsIntegration`, `TestSharedReservationsSurviveRestartAndRejectParallelOverspendIntegration`, `TestChildAdmissionCannotResetRootBudgetIntegration` and owner-day root preflight; admission precedes effects and survives worker/run/child reconstruction |
| 8. Worker facts remain candidates and head CAS wins | Actual Broker `memory.propose`, durable claimed dispatch, source graph and lost-ACK history; two worker candidates cannot replace a confirmed preference, and competing owner decisions on one current head yield one winner/one stale result |

The exact PG/Broker/budget/candidate stage passed60 names/37 top-level checks,
zero failures/skips, race and vet, with no inference. Proof:
`/srv/storage/butler/agents/resource-full-acceptance-20261012/reports/resource-matrix-20261012T080634Z-617544`.
Events SHA256 `784de2252dc4556ec6b542a20353d9dd3f6f1c79e4213b87295063f087d7bc80`;
source manifest `3f131ca894313c23a22169ad8f326ef8b796a966ffc3ec850d691aa5e95d3380`.
The earlier preparation omitted the actual candidate Broker application and
retained two opt-in skips. The corrected stage builds and invokes that production
application; skipped cases are not relabelled as passes.

Actual browser proof uses snapshot `20261012-151338-266`, source49b131a:
`/srv/storage/butler/agents/resource-full-acceptance-20261012/reports/single-tab-20261012T081342Z-642175`.
Its PG stage passed64 names/27 top-level checks; the one actual Chrome test was
opted out in the Go-only container and executed separately. The complete bridge
stage passed11 names/11 top-level checks; the command package had no test files.
Actual Chrome passed all ten scenarios in21.70s. Nine call the statically imported
production extension through a native-frame forwarder. The positive installed
case registers the real Linux native-host manifest and uses Chrome.connectNative,
actual owner metadata/atomic selection and private Broker, with no seeded selection.
Events: PG `198aa55dbe637d0de5864fd96c08283ed0c8d263e36ac445d96faf728dd3c7ab`,
bridge `83ceb44206177a88c2529954b34d1b01b8e5d0a7bbde7e4b9d2616a93cd56c9a`,
Chrome `9cfbd3197150c1e41b49e1b226b7f8da17096e063ef60096636984edbace80d0`.
The immutable snapshot's manifest, hashed after the run, is
`85bdd902856c02a381d6af5b7e1b7fddf61ac29f98b65da566508d755a8ed780`.
The earlier Chrome runner failed executable permissions before invoking the
test. The corrected runner exposes only its dedicated executable binary mount,
not private caches, to UID10001.

[Workspace proof](workspace-owner-discard-acceptance.md) includes real private
RPC/files, natural lease expiry, old Runtime subprocess/SIGKILL, row barriers,
independent absence, rollback/quarantine, lost erasure ACK and retained UNKNOWN.
Its final committed-source regression passed447 names/228 tops in11 packages
with race/vet/builds. Eleven explicit opt-in cases and their separate gates are
listed there; this is not a zero-skip broad-suite claim.
[Owner API/UI proof](task-owner-state-20261012.md) covers authenticated bounded
discovery, strict current CAS/body, exact delivery retry and explicit reload.
It does not manufacture physical erasure or external recipient evidence.

The stale unattempted request failed against the old kernel, then passed with
the restrictive fix84a96b9 (rootb0d0d85). Replacing it requires a distinct explicit
owner delivery, exact previous ID, actually changed task/constraints and every
current CAS, while resource/generation/source/root/old-lease proof stays exact.
Both owning code and068 trigger permit superseding a requested intent only with
NULL attempted_at. Original dispatch/effect/budget and old frozen request remain
retained. Old worker never revalidates it; old delivery retry cannot receive a
successor's permission or receipt. The PG fixture applies task/ownership changes
matching controller semantics; the separate actual owner HTTP/PG/Chrome proof
executes the real controls. Independent code review confirmed these restrictions.

Final integrated snapshot `20261012-154240-383`, rootb0d0d85, passed33 workspace
names/16 top-level tests (zero test skips/failures, race/vet) and75 owner
API/task/approval/Telegram names/67 tops (zero skips/failures, race/vet).
Workspace proof `issues-after-delegation/reports/workspace-20261012T084324Z-745593`,
events `6afdc993475a464e9b6ab8c68266a056b220e9bd77e920e7d209f1442fb94247`;
owner proof `issues-after-delegation/reports/owner-state-20261012T084324Z-745501`,
events `a4f23b4d71decb599f3c8aec00c2508b7745a70ffc24621c7e063d7f8863c6e9`.
The final broader447 test source is snapshot `20261012-154124-629`, proof
`workspace-owner-discard/reports/workspace-20261012T084128Z-738153`, events
`cadd413e12a2bb8e7714ddc1cfe9c9ece72b61f9addfc0d245cf9aef19e04a4d`.

Browser UNKNOWN remains quarantined in both PG and the extension journal.
`blockedTabReconciliation` deliberately denies clearing it without an exact
independent owning effect proof. Workspace discard never reconciles a browser
effect or grants a successor on the same tab. New independent work requires a
distinct physical resource selected by the owner and fresh task/source/budget
authority; a session/profile alias cannot remap the old lane. This is defined
safe failure/quarantine, not implemented same-tab owner unblocking or proof that
an external recipient received a click. Actual owner-account installation,
network-policy activation and default-off deployment flags remain separate.
