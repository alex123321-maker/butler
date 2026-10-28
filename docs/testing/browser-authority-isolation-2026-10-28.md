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

## Minimal Core rollout and actual native results

The deployed binary is b157b123215d9a7499551488054992c28a8fc41d9a3649bab79ba3e0fcee8614,
from47f9429c + GET evidence patch6cb3cff0 + authority delta59569a98 sourced at
cc4d7701. Only the six relevant production files and two tests were backported;
the current HEAD's other Core changes/migrations were not rolled out. The source
manifest pins all layers. Actual minimal-build PG/race checks:59 selected
tests/subtests PASS,0 skips/failures; vet/build PASS. An earlier source-check
failed on an unrelated test-helper context; that failed bundle/timing is retained.
The corrected minimal patch excludes that helper and passes the owning tests.
The final SSH wrapper had an exit-variable quoting error after successful build;
read-only inspection confirmed all phase exit codes0 and the binary SHA, without
another build/test run.

Core-only rollout passed exact old/new hashes, health, identical environment
and unchanged Broker/browser/PostgreSQL identities. Live allocation reports
total4 = business2 + authority1 + source1. No migration or provider/account/cap
change occurred; old UNKNOWN effects and reservations remain. Prior Core/binary
and private Compose/environment rollback are retained. The active override is
/srv/storage/butler/main/releases/browser-authority-20261028-cc4d7701/private/candidate.compose.json,
in addition to the existing base and Broker/browser override; keep all layers.

| Sample | Answer + Telegram | Telegram, s | Input, s | Model attempts | Input/output tokens |
| --- | --- | --- | --- | --- | --- |
| authority-isolation-01 | correct/delivered | 131.808 | 144.427 | 15 | 169907 / 1255 |
| authority-isolation-02 | correct/delivered | 107.821 | 112.586 | 15 | 153746 / 1128 |
| authority-isolation-03 | correct/delivered | 105.264 | 109.969 | 14 | 150094 / 1337 |

All three task answers/private-recipient checks and actual deliveries passed,
with no in-run owner intervention, no authority-failure log entry and known token
pairs. However a private existing-observation audit shows01/02 obtained the
exact record only through HTTP;02 did not finish the guarded browser search.
Only03 observed it in the browser. Thus **task answer/delivery3/3, full-browser
record coverage1/3**, not a general3/3 browser qualification. The audit causes no
new website/model call and exports booleans only. The previous failed sample
remains failed with both NULL usage pairs.

Preselected goals: Telegram median107.821s passes<=110; input-token median153746
passes<=160000; model-attempt median15 FAILS<=14. Monetary cost is still unknown.
All inputs completed but browser-only coverage is insufficient, so the overall
browser qualification remains NO_GO. Do not infer causal speedup/p95 or a proven
retrospective source of every authority timeout from three outcomes.

Per-step/action/model-accounting/state/delivery timing for all20 existing trials
is exported without replay; see browser-action-timings-2026-10-28.md and
browser-baseline/timings-summary-20/. Operator phase timings also retain failures
and clock semantics. New build/rollout stages were measured prospectively;
previous unmeasured stages were not rerun to fill blanks.
