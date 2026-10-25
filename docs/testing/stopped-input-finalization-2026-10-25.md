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
