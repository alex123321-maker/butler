# Full High fixture expiry after a PostgreSQL row wait

Scope: the isolated full High acceptance fixture, based on `8ee07e7`.
The production provider and production budget deadlines are unchanged. This
fixture still carries 27 historical attempts, admits at most 12 new attempts,
and retains its immutable 18-minute window. Existing historical reports and
native windows were neither edited nor reopened.

`reserveFullHigh` now reads `expires_at > clock_timestamp()` again while holding
the counter row lock, before inserting and committing an admission. A failed
fresh check returns the fixed fixture denial and rolls back the provisional
counter update. PostgreSQL can evaluate an UPDATE predicate before waiting for
an unchanged row lock; when the locker rolls back, that predicate need not be
reevaluated. The regression establishes this behavior on PostgreSQL 17.

`TestFullHighDelegationExpiryAfterUnchangedRowWaitIntegration` shortens only its
isolated schema's deadline before the race, locks the unchanged counter using
`FOR UPDATE`, and observes the actual waiting UPDATE through
`pg_stat_activity`/`pg_blocking_pids`. It waits for natural expiry according to
the database clock, then rolls back the locker without modifying that row.
The two-connection pool's locker performs all wait observations; there is no
third-connection deadlock or production clock hook.

The request is valid and passes through `limitedNativeDelegationProvider` to a
concrete Antigravity provider with valid synthetic Cloud Code authentication.
For provider construction only, this serial test explicitly replaces and then
restores `http.DefaultTransport`; the provider clones the scoped transport.
HTTPS is served through `net.Pipe`, with no network dial or external endpoint.
That permits the old implementation's erroneous entry to be counted safely.
The corrected implementation must deny before authentication or HTTP entry,
leave the counter at 27, and leave the attempt ledger empty. Synthetic provider
output, request bodies and authentication material are not printed.

## Red and green evidence

All checks ran on SSH `laba`, Docker Go 1.27.2 and pgvector/PostgreSQL 17, using a
new disposable internal Docker network and test database. No imported profile,
native authorization flag or actual model call was used. Caches remain within
the lab repository; reports and snapshots use the dedicated agent root
`/srv/storage/butler/agents/delegation-high-expiry`.

The original guard, plus the regression, failed as expected:

- Snapshot: `20261012-114320-037`.
- Report: `reports/expiry-red-20261012T044420Z-4130981/tests.jsonl`.
- SHA-256: `c12fccd8bbacd94f75d6bcd4c22d69b0babd37e2cfe14217c4ca5104378742e5`.
- Evidence: attempts 28, ledger rows 1, synthetic authentication entries 1,
  synthetic HTTP entries 1 after natural expiry and unchanged-row release.
- The other three narrow checks passed; no check skipped.

The corrected guard passed the same checks:

- Snapshot: `20261012-114503-048`.
- Report: `reports/expiry-green-20261012T044518Z-4134678/tests.jsonl`.
- SHA-256: `e900b6138f46f94f1c384be0c556c87f7fa5f6b4a228d9e0c9ded0b6302fdb48`.
- Four passing top-level tests, zero failures, zero skips; `go test -race`.
- `go vet ./apps/orchestrator/internal/app` passed without network access.
- Checks: `ExpiryAfterUnchangedRowWaitIntegration`, `History`,
  `LimitIntegration`, and `RevalidationIntegration`, each prefixed with
  `TestFullHighDelegation`.

This evidence establishes the fixture admission boundary after a row wait.
It does not establish native model acceptance or explain the older parent
receipt failure; those remain separate gates for the owner-authorized scenario.
