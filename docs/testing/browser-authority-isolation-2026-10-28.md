# Independent foreground source and authority capacity

Issue #174 follows the retained third-sample failure from PR #173. Its exact
Input/run and NULL usage remain in browser-baseline/core-evidence/. Two actual
watchdog failures reported lease/deadline/750ms before any browser call. The
live failing instant did not record which query held the shared reader; source
contention is a demonstrated design hazard, not a proven retrospective cause.

## Implemented boundary

Core separates longer token-only source observations from lease/Task authority
without increasing the configured PostgreSQL connection cap:

| Total maximum | Business | Authority | Foreground source |
| --- | --- | --- | --- |
| 3 | 2 | 1 | business fallback |
| 4 | 2 | 1 | 1 |
| 10 | 8 | 1 | 1 |

Minimum counts also preserve the configured total, including DSN-only limits.
The minimum3 configuration remains supported; a busy business pool can deny its
source observation, without consuming lease capacity or authorizing stale data.
Source writes and compound/converter guards retain business transactions. The
existing bounded local-model watchdog retains its original authority reader.
Nothing changes the750ms lease/Task bound, source bound, current database proof,
fencing/expiry, task/source revocation, retry allowance or effect/spend state.

Lease errors retain errors.Is/As causes while exposing a fixed public string.
Failure logs add pool_acquire/query phase and acquired/idle/total counts only.
No SQL, parameters, DSN, raw error, credential or lease identifier is logged.

## Actual isolated verification

Linux Go1.27.2/race and disposable pgvector17 passed required cases without skips:
held source connection reproduces a timeout through the old shared route;
independent live lease/Task checks still work while both source and business
capacity are occupied. A source check cannot borrow authority capacity. An
exclusive lock inside the disposable schema independently proves SQL-query
timeout diagnostics, distinguished from pool-acquisition timeout. Subsequent
pause/expiry/deletion-anchor tests still deny current authority/source use.
Actual cap/minimum/DSN cases3/4/6, durable two-connection lease fencing,
watchdog cancellation/retry and separate Core/Broker request-scope checks pass.
Vet passed. No production database or real model/channel was used.

Command: scripts/dev/browser-authority-tests.sh OWNED_ROOT. Required cases fail
on a skip. The ordinary worker/traced fixtures now mirror8+1+1 instead of9+1,
keeping their total ten-connection allowance.

## Frozen next native window

After a reviewed minimal Core build and health/environment/peer checks, exactly
three fresh controls are preselected: browser-baseline-authority-isolation-01,
02 and03. Use the same Russian public-search prompt, owner/session/model and
current caps, fresh recipient/config/context, and no tool hints or in-run owner
intervention. Keep core-evidence-03 failed with both unknown usage records.
New trials never replace old ones and do not represent original matrix slots.

Qualification:3/3 exact record + protected publication + current-run browser
receipt + Telegram sent, with no authority observation failure. Retain the
earlier performance goals (median Telegram<=110s, model attempts<=14 and input
tokens<=160000); do not increase them after seeing outcomes. Monetary cost is
unknown; report any failed/censored sample and incomplete usage explicitly.
The experiment can show a small observed outcome, not a reliable p95 or proof
that every past authority timeout was caused by the source pool.

POST/login/consent remain unaccepted under the retained unresolved browser
write. This capacity fix neither reconciles that write nor grants new authority.
