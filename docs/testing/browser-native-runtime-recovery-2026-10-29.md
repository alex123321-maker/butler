# Native browser-runtime recovery — 2026-10-29

**Bounded assisted recovery passed in `runtime-recover-07`.** A real SIGKILL of
the browser runtime before the first local preparation retained the same Input,
run, pending approval, history and usage. The model opened a distinct context,
submitted exactly one form, read its control reference, published the correct
answer, and completed the Input/run. Web has `result_ready`; Telegram has `sent`.

Inclusive native Input time: **360.370s (6m0.370s)**. This includes model reasoning,
context, scheduling, fault injection, operator approvals/reporting and tools.
Assistance: **five exact public-action approvals and one factual operator report**
for the opaque POST. Two HTTP reads supplemented actual browser observations.
This does not establish autonomous reconciliation or browser-only execution.

The [joined qualification](browser-baseline/native-runtime-recovery/runtime-recover-07-qualification.json)
checks the independent evidence below. No raw answer, reference, credentials,
cookies or owner token is exported.

| Criterion | Evidence |
| --- | --- |
| Actual process loss; same binary/Input/run/history/usage; other services unchanged | [process proof](browser-baseline/native-runtime-recovery/runtime-recover-07-process-loss.json) |
| Lost preparation ABSENT, consumed once; two distinct context hashes; original UNKNOWN click transcript retained | [durable audit](browser-baseline/native-runtime-recovery/runtime-recover-07-final-audit.json) |
| One accepted POST, quantity 3 | [recipient journal](browser-baseline/native-runtime-recovery/runtime-recover-07-recipient.jsonl), [form verification](browser-baseline/native-runtime-recovery/runtime-recover-07-form-verification.json) |
| Exact title, quantity and saved result in protected answer | [private text comparison booleans](browser-baseline/native-runtime-recovery/runtime-recover-07-report-check.json) |
| Correct reference observed through browser and HTTP; present in answer; no credential canary | [observation comparison](browser-baseline/native-runtime-recovery/runtime-recover-07-observation-audit.json), [answer verification](browser-baseline/native-runtime-recovery/runtime-recover-07-read-verification.json) |
| Explicit owner factual report, not machine receipt | [report receipt](browser-baseline/native-runtime-recovery/runtime-recover-07-owner-outcome.json) |
| Input/run completed; Web/TG states; all 22 usage pairs known | [terminal metadata](browser-baseline/native-runtime-recovery/runtime-recover-07-metadata-final.json) |

## Changes

Revision4 local preparation adds a closed Go-only pre-input session-loss proof.
Navigation revision4 independently proves rejection before GET for a Go-observed
lost or frozen context. Broker/PG verify exact revision/policy, correlation,
closed proof and consumed dispatch. The resource wrapper forwards a rejection
only after current authority and durable Finish, without a success ACK or new
session mapping. In-flight loss, opaque failures and lost Finish remain UNKNOWN.
The model must propose a new open and reassess; no old grant, DOM or credential
state is restored and no previous external effect is promoted.

Core retains the five-second frozen-observation dispatch deadline. After a
correlated response returns, that child is cancelled and persistence alone gets
up to three seconds from the original live run context. It cannot dispatch tools,
extend the parent or survive its cancellation. Storage failure still blocks.

Exact terminal compatibility pins cover revision3 navigation and revision1
Broker-owned catalog search. Owning review can establish only no-write
uncertainty. Historical read result, effect, grant and dispatch remain UNKNOWN;
old Input/run remain failed. Foreign hashes/revisions, credentials, live leases,
wrong policies and unsupported catalog operations remain denied. Review runs
dry first, then commits append-only proof; there is no direct classification edit.

## Qualification and CI

Isolated PostgreSQL and actual Chromium passed under race without required skips:
pre-input process loss, fresh context plus one POST; frozen navigation with zero
target GET before a fresh successful read; the three earlier runtime-loss cases;
and all ten existing resource regression parents. Full-source and actual narrow
Core observation units passed under race; vet and production builds passed.

Evidence under `browser-baseline/native-runtime-recovery/`:

- `build-20261029T070500Z-1631866`, `integration-20261029T070725Z-1647374`: preparation.
- `build-20261029T074438Z-1806585`, `integration-20261029T074632Z-1815715`: navigation plus preparation regressions.
- `observation-persistence-20261029T081716Z-1959243`: full/narrow Core units and build.
- `stopped-navigation-20261029T080358Z-1889546`, `stopped-catalog-20261029T085458Z-2156473`: exact pins, PG/race and standalone review operator.
- `source-plans-20261029T085145Z-2140561`: existing summary/temporal predicate equivalence, invalidation and down/up PG/race.

Both `49ada352` and code commit `1ea37b7b` finished **11/11 CI checks**, including
both PostgreSQL communication matrices, credential/real-Chromium recovery and
joined owner UI acceptance. Snapshots: [previous CI](browser-baseline/native-runtime-recovery/ci-49ada352.json),
[observation-fix CI](browser-baseline/native-runtime-recovery/ci-1ea37b7b.json).
Later catalog-pin/report commits have their own PR checks; a prior snapshot does
not assert that a later SHA has finished. Final status is reported on PR177/#163.

## All additional native attempts

Each used ordinary canonical owner ingress and the actual configured
Antigravity/gemini-3.8-flash-high provider, with a fresh receiver identity/config.
None replaces a historical slot or replays an old Input for measurements.

| Attempt | Input/run outcome | Inclusive seconds |
| --- | --- | ---: |
| 01 | Failed `memory_source_check_timeout`, before any fault or POST | 131.110 |
| 02 | Process recovery and one POST; subsequent legacy navigation UNKNOWN; no answer | 472.841 |
| 03 | Process restarted; new preparation denied by earlier stopped uncertainty, zero preparation dispatch | 595.992 |
| 04 | One POST and confirmed frozen read; observation checkpoint failed before owner wait | 341.436 |
| 05 | External criteria and Web/TG delivery passed; run then failed lease renewal | 596.785 |
| 06 | Third catalog search UNKNOWN before browser open/fault | 408.214 |
| 07 | Assisted complete task, Input/run and Web/TG delivery after actual browser-runtime loss | 360.370 |

Attempt01's exact source-timeout cause is not proven. Its two-second source guard
was not weakened. Attempt02 retained a revision3 UNKNOWN read; its exact owning
no-write review released isolation without making the read/run successful.
Attempt03's denial was correct. NOWAIT review contention failures are retained;
only rolled-back lock conflicts were retried boundedly.

After attempt 04's context expired at **08:23:26.022144 UTC**, the protected
historical-review API accepted a factual report from its one-POST journal. Its
Input/run remained failed; no historical execution resumed. Attempt05's correct
answer was delivered at 546.051s, but `execution_lease_renewal_failed` then stopped
its run. The exact renewal failure phase/root cause remains unproven. Attempt06's
UNKNOWN catalog read likewise retains its original result/spending/failure after
no-write review. Unknown usage pairs remain unknown, not zero.

The original 15-slot matrix remains **3 read completions, 1 assisted form completion
and 11 failed complete-task slots**. General browser acceptance remains **NO_GO**;
#163 stays open. This qualification covers pre-input process loss with ordinary
approvals and an explicit factual POST report. It does not establish arbitrary
mid-POST recovery, autonomous outcome verification, popup SSO, CAPTCHA or WebSocket
support. No PR merge or full-branch deployment is implied.

## Inclusive timing

[Raw timings](browser-baseline/native-runtime-recovery/timings/) and
[per-action CSVs](browser-baseline/native-runtime-recovery/inclusive-actions/README.md)
derive from existing records only: **7 trials,88 tool spans**, all wall-time
partitions complete. Each action interval starts at the previous tool completion
(Input acceptance for the first) and ends at this tool completion. It includes
model deliberation/context, scheduling, approvals/operator waits and the tool.
Gaps are not isolated model-only latency. Finalization is retained separately.

For attempt 07, tool spans total **6.877s**, intervening decision/context/wait
intervals **346.234s**, and finalization **7.259s**, summing to 360.370s. Web was
available at 352.855s; Telegram sent ACK at 354.350s. Delivery overlaps execution
and is not added to Input time or described as a human-read receipt. There were
22 model attempts,22 recorded tool spans (including two locally rejected drafts),
**291,277 known tokens** and zero unknown usage pairs. Monetary cost is unavailable.

The inclusive operator work window from the first recorded **06:41:58 UTC** to
verified native qualification at **09:08:08 UTC** was **8,770s (2h26m10s)**,
including Codex reasoning, inspection, implementation, failures, builds, rollouts,
waits and all trial verification. This is separate from native Input latency.
Time before the first timestamp is unavailable. Documentation and remaining CI
after this boundary are reported separately. See [work window](browser-baseline/native-runtime-recovery/inclusive-operator-window.json).

## Lab deployment and remaining limits

The live binaries are narrow actual-lab backports, not the whole PR source:
Core `2ba13a140622ef06b1889e20a119a0c51ff36ee4b572cb0eb6bc1ede6a99358a`;
Broker `dc020e2c7d663e2cc6d8c1a50286efcbbf96670cde80df6390dbb2058cf6e863`;
browser `e58843f27b154d32d88c4e4c78d09fae5a5d4f68427ee5ea40cde75d84c49276`.
Rollouts v12/v13/v14 retain their separate metadata. The new owner-review UI is
not deployed. Live resource arbitration is off; the guarded wrapper is qualified
separately through actual isolated PG/Chromium. Provider/model settings, credentials,
PIN, grants and budgets were not reset. Inference/source/authority deadlines remain.

The live DB had 105 but lacked existing migrations103/104: summary/temporal
currentness still used original SQL functions. After original-predicate
equivalence tests, the existing plan migrations were applied atomically under
the migration advisory lock with ledger rows and private rollback definitions.
All service containers stayed unchanged. They retain query plans/current statement
snapshots, never accepted facts or authority. [Before](browser-baseline/native-runtime-recovery/source-plan-before.jsonl),
[deployment](browser-baseline/native-runtime-recovery/source-plans-apply.json).
Narrow summary/temporal probes afterward took 2.623/0.309/0.419ms; PostgreSQL still
showed substantial background CPU. These query-only observations do not prove
that all lease/capacity failures are solved. A guarded-source generic-plan probe
measured 21.573ms with JIT enabled versus 21.055ms disabled, with no JIT selected;
no JIT or pool setting was changed on that unproven hypothesis.

The host retains severe storage/swap pressure. The first build failed on noexec
tmpfs; later tests used private exec tmpfs and caches. Owned retired binaries
were gzip-preserved only after verifying they were unmounted and checking the
decompressed hash. An old source archive was hash-verified and retained through
a host `/dev/shm` symlink; this is volatile retention, not a production mount.
Active binaries remain on durable read-only release mounts. No broad prune or
production history/log deletion occurred. Full-root source/log evidence is under
`/srv/storage/butler/agents/browser-runtime-native-20261029/reports` on `laba`.
