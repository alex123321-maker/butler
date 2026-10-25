# Stopped Input finalization, 2026-10-25

This implements the requested separation of failed attempt lifetime from retained
action uncertainty. An inactive stopped Input/run can end with failure and free
the conversation while its unknown journal entries remain guarded. It never
retries the stopped request, executes its tools, fills unknown usage or refunds
spending. See [the implemented contract](../architecture/stopped-input-finalization.md).

## Verification

Windows narrow checks and changed-path spec lookup passed. Linux used Go 1.27.2,
disposable PostgreSQL 17 and `-race -count=1` in the task-owned workspace
`/srv/storage/butler/agents/research-recovery-20261024`.

The full owning-package regression passed 841 tests including subtests across
budget, action authorization, Input, execution notices and Communication. The
separate native Telegram outbox process-restart test was skipped because its
opt-in runtime was not configured; its implementation is not changed here.

Final targeted checks passed 74 tests including subtests with no skips, plus
two app integration tests for actual owning Telegram ingress and public payload
anti-forgery. `go vet` passed for all affected owning packages/app. Core and
Broker builds passed. A SHA-256 manifest matched all 22 changed Go/schema files
to the exact exported source used by the final checks/builds.

The new cases exercise:

* Known/unknown conversation failures, explicit Task uncertainty and historical
  canonical admissions without upgrading old transport epochs.
* Already failed attempts acquiring retained isolation proof; missing journal
  receipts remain uncertain even when a generic runtime says completed.
* No takeover of live leases/claims, pending owner questions, completed runs,
  recent state changes or locked journal rows.
* Atomic rollback across Input, Run, obligation and audit; one failed row does
  not prevent another stopped attempt from being finalized.
* Concurrent sweeps producing one audit, and separately established journal
  reconciliation clearing isolation without reopening the failed attempt.
* Unchanged journal bytes and retained NULL model usage/reservations.
* A fresh worker automatically processing the original queued Telegram Input,
  with no second ingress or old executor invocation.
* Actual Telegram owning ingress reproducing the blocked queue and reclaiming
  the second original update after store restart, retaining the old unknown effect.
* Receiving grant claim denying another write without dispatch or partial charge;
  a changed tool/Task does not evade the same-session guard.
* Fresh owner text replies requiring exact live Input/intent/admission and
  canonical grant metadata. Expired claims, exhausted publication allowance,
  mismatched arguments, untracked/missing provenance and equivalent unknown
  replies are denied. Independent authorized reads remain possible.
* No repeated identical Telegram failure after automatic terminalization, while
  Web retains the current failed state.

These are real PostgreSQL/worker/owning-ingress/authorization boundary tests with
synthetic updates/providers/recipients, not a fault injected into the working
database or proof that every external provider can be reconciled automatically.
Tasks and legitimate waiting_for_user flows retain their independent lifecycle.
Missing run/ownership proof remains fail-closed. Non-read operations remain
restricted while action uncertainty survives; fresh text replies are the narrow
additional exception, not blanket write authority.

Private raw evidence: `reports/stopped-input-regression.jsonl`,
`reports/stopped-input-final.jsonl`, `reports/stopped-input-telegram.jsonl`,
`reports/stopped-input-final-vet.log` and
`reports/stopped-input-source-sha.json`. Disposable test containers/network were
removed. Old task-owned source exports and rebuildable Go object archives were
pruned after a candidate build hit storage capacity; deployed releases, source
Git history, database state and evidence reports were retained, and both binaries
were rebuilt successfully afterward.

## Scoped deployment and native reply

Core and Broker code `619b99d7b1773d88959bcf6189733157182d2a95` was deployed on
`laba` to `/srv/storage/butler/main/releases/stopped-inputs-20261025-619b99d7`.
Migration102 applied before the new Broker/Core composition started. Other
runtimes, operator policies, profiles and budget settings were retained; the
Compose backup is private. Health returned 200. Running `/proc/1/exe` digests
matched the built binaries:

* Core: `102577271332c7706b687c6426c08d48adc958fb613b1d3a7f8a85caec0a892f`
* Broker: `6c1315b865cc0522b93c29352060aa10e5a8838b21ddde1fd4417b188deb6d85`

The sweep registered 24 known and five uncertain historical attempt failures,
with the latter retaining their action uncertainty. A new Web owner request,
Input `input-7132319dd490573fb3fa16cd15586b69`, run
`run-1792900505580556062-74583aa8fc2ca9ba`, then published the ordinary result
“Я на связи.” through `communication.message`. Four uncertain stopped attempts
remained in that same session. The old
`action_56a8dba2404859ee5e7350e77f5eeb52519c1a0d4ddd822a0d48614789fc5d1e`
still had state `effect_unknown`; it was not replayed or reconciled. The fresh
reply's effect was confirmed and did not use the older blocked-notice flag.

Protected Web read returned 200; Telegram acknowledged message 175. Two model
attempts retained 24,283 measured input and 38 output tokens, with zero unknown
usage pairs. This native smoke used Web ingress and actual Telegram delivery;
the dedicated owning-Telegram ingress/restart tests above used synthetic updates
against real PostgreSQL, without injecting a failure into the working service.

Additional private reports: `reports/stopped-input-deployment.json` and
`reports/stopped-input-native.json`. The release remains on draft PR #150;
main was not merged.
