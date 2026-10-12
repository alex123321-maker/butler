# High acceptance: two remaining attempts

This fixture limiter carries all 37 recorded native attempts and permits only
ordinals 38 and 39. The original total ceiling remains 39; this does not reopen
the prior windows, refund calls, or add another 12-call budget. Production
owner/root/day accounting remains independent and mandatory.

The explicit fixture selector is `BUTLER_DELEGATION_HIGH_REMAINDER_WINDOW=1`.
Shared provider routing and the actual scenario remain the caller's responsibility.
The helper API is `newHighRemainderDelegationLimiter(ctx, pool)` and
`(*nativeDelegationLimiter).reserveHighRemainder(ctx, kind, runID, input)`.
Each start, continuation or tool-result entry validates all eleven historical
artifacts again before attempting a durable reservation. The fixed same-source
paths do not come from a model or environment-selected artifact path.

The first ten artifacts retain their existing readers and bounds. The eleventh,
`deploy/delegation/native-acceptance-high-20261012.json`, is pinned to SHA-256
`afa63de06b6991feb3a96b322d442d573d4d2834c51a70c07e5fd4419f751a91`
and release `b2100bd2bcd4147b1168a252cd7369c3415c5140`. Its 43,072-byte
content-free audit requires a new reader capped at 64 KiB, rejecting nonregular
paths, symlinks, mismatched opened inode, missing bytes and wrong hash. Existing
historical readers keep their 32 KiB limit.

The carried report remains failed. Both native parent receipt gates passed and
all four native child observations passed. Width1 retained the independent
originals, authenticated owner decisions and unverified model finals. Width2
recorded overlap one at its `parent_receipts_checked` stage and did not reach its
later independent checks; Stop/Amend/Delete acceptance flags remain false. The
reader checks these exact scopes along with the hash and historical nesting;
it does not claim the unexecuted gates passed.

Usage carries 37 entries: 17 known pairs and 20 unknown pairs. Known totals are
62,760 input tokens and 1,810 output tokens. Unknown values remain null and are
never converted to zero or charged as new fixture usage. The baseline hash
extends the existing chain with the exact eleventh report hash.

PostgreSQL stores a counter with baseline 37, total cap 39, new cap two and an
18-minute deadline. The trigger forbids deleting the row, decreasing the counter
or altering its limits, baseline or deadline. Reconstruction preserves both
spending and the initial deadline. A fresh database-clock check after acquiring
the counter lock denies a reservation that waited beyond natural expiry; the
transaction rolls back its provisional counter and attempt row.

The remaining scenario must explicitly label scripted parent/component work
separately from its two native width2 child starts. This limiter alone establishes
spending admission, not inference, parallel execution, owner completion or the
remaining controls. Combining this retained failed report with a subsequent
parallel/control proof must preserve those distinct scopes.

## Isolated validation

Source base: `b2100bd`. Only the new limiter/test file and this document belong
to the change. The exact root-owned eleventh artifact was copied for validation
and is committed by the parent task separately.

All checks ran on SSH `laba`, Go 1.27.2 in Docker and PostgreSQL 17 with pgvector,
on a new disposable internal network. No native authorization flag, imported
authentication or provider call was used. `go vet` ran with network disabled.

- Agent root: `/srv/storage/butler/agents/delegation-high-remainder`.
- Snapshot: `20261012-121918-017`.
- Report: `reports/expiry-green-20261012T051946Z-56601/tests.jsonl`.
- SHA-256: `6b74003f59b812d360b0f43c63a285db903739bb961cadf18068597e28158bb4`.
- Four passing top-level tests with `-race`, zero failures and zero skips.
- `go vet ./apps/orchestrator/internal/app` passed.

`TestHighRemainderDelegationLimitIntegration` races sixteen admissions for exactly
two slots, checks ordinals 38–39 and reconstruction, and denies immutable changes.
`TestHighRemainderDelegationRevalidationIntegration` denies missing and tampered
bytes for each of eleven pins on all three entry kinds, invalid inputs and an
expired deadline without spending. `TestHighRemainderDelegationHistory` checks
the carried scopes/accounting and incomplete-chain denial.
`TestHighRemainderDelegationExpiryAfterUnchangedRowWaitIntegration` observes a
real unchanged-row lock wait, waits for natural database expiry, rolls back the
locker and proves counter 37/empty attempt ledger after denial. The test shortens
only its isolated schema's deadline before starting; it uses no production clock
hook and no third pool connection.
