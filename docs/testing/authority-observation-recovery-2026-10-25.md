# Lease-observation timeout recovery, 2026-10-25

The previous research probe stopped when the owning watchdog could not read its
lease within 750ms. That attempt remains closed and its unknown usage is retained;
it was not retroactively resumed or reclassified.

Core now cancels the interrupted model context immediately, then can queue one
durable continuation of the same accepted Input and nonterminal run. The trusted
lease-read deadline shares the existing nominal model-timeout retry allowance.
Fresh task/source/claim/fence checks precede the CAS. Backoff is two seconds,
proof collection is bounded to five seconds under the live Input parent, and
the saved original worker deadline caps the resumed context. All reservations
remain spent. Unknown actions, revoked authority, stale claims and an exhausted
allowance deny continuation. A second observation timeout retains a finite
timeout diagnosis for normal execution-state delivery.

## Verification

Windows narrow checks used `scripts/dev/test-go.ps1` for the owning
orchestrator/input packages; `scripts/dev/verify-changed.ps1` mapped the affected
specs. Linux used Go 1.27.2 and a disposable pgvector/PostgreSQL 17 instance in
the task-owned workspace `/srv/storage/butler/agents/research-recovery-20261024`.
The test database and network were removed after the run.

The targeted `-race -count=1` run passed 21 top-level tests, 119 including
subtests, with no failures or skips. `go vet` passed for both owning packages.
The test-only lease wrapper stalls the actual watchdog observation until its
750ms context expires; PostgreSQL ownership, Input claims, checkpoints, model
reservations, task validation and ordinary worker/Core execution remain real.
Providers and the external-action adapter are scripted; these are integration
tests of recovery, not a fault injected into the deployed service or proof that
a real provider will always succeed on its second attempt.

Additional full regression of the two owning packages passed 470 tests including
subtests under `-race`. One legacy Redis-backed test,
`TestExecuteIntegrationPersistsRunAndTranscript`, was skipped because its Redis
URL was not configured. All required recovery cases ran against PostgreSQL.
That report is `reports/authority-retry-regression.jsonl`.

The new cases cover:

* Same Input/run recovery after constructing a fresh worker/store/lease manager,
  both with an explicit Task and in a taskless conversation.
* Recovery after a confirmed action; exactly one preparation and dispatch.
* A new-ID equivalent write denied before preparation.
* A repeated watchdog timeout stops after the one allowed continuation.
* Changed sources, stopped tasks, exhausted budgets and the original worker
  deadline reject another provider request.
* Superseded leases, expired/out-of-range deadlines, exhausted Input claims and
  unknown effects deny the durable CAS.
* The nominal and observation timeout reasons share one counter in both orders.
* Provider text, an unrelated cancellation cause and a joined independent
  transport failure cannot qualify; original unknown usage stays NULL.
* Successful recovery stores one final assistant result; blocked cases store none.

Raw reports: `reports/authority-retry-race.jsonl` and
`reports/authority-retry-vet.log` in the task-owned workspace. No old reservations,
unknown effects, deadlines or acceptance records were reset for these checks.

## Scoped deployment and native delivery smoke

Core commit `359ef27b2816a30a009ad742d5bea970b427b7a8` was deployed on `laba` to
`/srv/storage/butler/main/releases/authority-recovery-20261025-359ef27b`.
Only the Core candidate mount/container changed; no migrations or configuration
relaxation were required. The previous Compose snapshot remains private (0600).
Health returned 200 and the live `/proc/1/exe` matched the built Core SHA-256:
`c7c79186a3cdd042c32f0a15353d31347386a879a0526bfd9f040c2d8d12197c`.
This remains a deployment from draft PR #150, not a merge into main.

One native taskless owner request checked the current Git release on its official
site after the restart. Input `input-80c8d65eb8c5e98266a183b5ca35a904`, run
`run-1792895742390752633-f81a60b808b47a86`, completed with a concise two-line
answer and a current-run primary page quote. No approval was requested. The
protected Web publication read returned 200 and Telegram acknowledged message
167. Five actual model attempts retained measured usage (28,514 input and 300
output tokens), with zero unknown usage pairs. This smoke validates ordinary
execution/publication/delivery on the deployed binary; the watchdog fault itself
was exercised by the isolated recovery tests above, not forced into production.

Additional private reports: `reports/authority-retry-deployment.json` and
`reports/authority-retry-native.json` in the task-owned workspace.
