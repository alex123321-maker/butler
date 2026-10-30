# Page binding ledger for #164 — 2026-10-30

Scope: migration107 and the private `browsercontrol.Store` page ledger. This is
component acceptance, not runtime/Broker dispatch, owner UI, native SSO, Google
or response-delivery acceptance. Production popups remain disabled.

## Local PostgreSQL/race evidence

The new implementation ran against a dedicated Windows PostgreSQL 17 cluster
under repo-local `.cache/browser-pages-local/data`, loopback port55443 and
`browser_pages_test`. It used existing PostgreSQL binaries; no shared database,
lab service, credential or provider was modified. Tests use separate random
schemas and clean them with DROP SCHEMA after execution.

`go test -race -count=1 -json ./internal/browsercontrol` passed with PostgreSQL
enabled and **zero skips**. Final local package wall time: 3.564 s. `go vet`
also passed. Five required PostgreSQL parents and their measured test times:

| Test | Wall time | Evidence |
| --- | ---: | --- |
| Lifecycle | 0.17 s | pending denial; own-page URL after Store recreation; changed-document/foreign-document denial; close/loss; sanitized persisted callback URL |
| ContextFences | 0.16 s | wrong owner/Run/Task/context/epoch; owner/expired/closed contexts; each page separately fresh after return |
| ConcurrentCapsAndCAS | 0.29 s | eight concurrent registrations admit only three children; eight observations admit one winner; terminal identities count toward lifetime cap |
| OwnerTransitionRace | 0.22 s | three operations actually wait on a PostgreSQL owner row lock; committed takeover denies all old operations without metadata change |
| RetentionAndDowngrade | 0.15 s | empty down/up; retained-state downgrade refusal; DELETE/TRUNCATE/identity/version rewrite denial; lost page cannot revive |

The race test requires actual `pg_stat_activity` lock waits before committing
takeover, rather than inferring contention from a scheduling delay. The context
tests exercise database epoch/mode transitions; they do not claim an owner UI or
runtime takeover occurred. Unit tests additionally validate exact references
and callback query/fragment stripping. Existing private owner protocol tests in
the package also passed.

The first result-export PowerShell command failed on a missing optional JSON
`Output` property under StrictMode after Go tests passed. The corrected export
uses hashtables; the full profile was rerun after adding the actual row-lock race.
The implementation did not require a change for that operator parsing failure.
An earlier profile also passed the four preexisting new PostgreSQL parents.

## Required CI and operational limits

`scripts/dev/browser-page-bindings-tests.sh` requires a disposable `_test` DSN,
runs the package with race instrumentation, rejects skipped/missing required
parents and runs vet. The Browser SSO workflow adds a PostgreSQL job and first
applies **all ordered production migrations** through the existing credential
integration test. It also keeps the five real fixture Chromium cases mandatory.
Actual CI conclusions are recorded in the PR/issue after execution, not inferred
from the workflow definition. Reports contain test names, elapsed times and fixed
outcomes; no live credential or provider inference is involved.

This ledger serializes metadata under the existing context row. It does not
grant execution, release credentials, check current task/mandate revisions,
admit initial popup traffic, inspect DOM, close an executor, clear UNKNOWN,
alter budgets or create another resource lane. A sanitized URL is still subject
to ordinary authority and egress checks in the future action composition.
See [the implemented status and remaining contract](../architecture/browser-pages-proposal.md).

## Inclusive operator timing

The first recorded timestamp in this continuation is **07:40:17 UTC
(14:40:17 Tomsk)**. The inclusive work window covers model deliberation, source
inspection, design, implementation, local setup/tests, CI and reporting. Final
checkpoint timing is added to the issue/PR after CI. Per-test values above
contain zero model calls and must not be presented as native agent latency.
No historical native login attempt was replayed for metrics.
