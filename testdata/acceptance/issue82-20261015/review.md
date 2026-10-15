# Independent review record

Reviewer: separate `review82` agent, read-only implementation/evidence audit on
2026-10-15. No independent inference or changes to main deployment.

Reviewed production commits `d99f5b7`, `6f7c897`; window scripts through
`a094a68`. Compared nine critical local/remote file hashes and read original
laba test logs, native receipts and live read-only PostgreSQL accounting.

| Finding | Resolution and evidence |
| --- | --- |
| Native adapter checks authority twice; a single-use marker in that guard would block every real call. | Separate repeatable authority and single dispatch hook; actual native adapter/HTTP regression passed. |
| SQL failures could be normalized into terminal source denial. | Preserve actual storage failures in catalog/authority helpers and request-local private worker cause. SQL40001 before HTTP and during publication recover with no premature HTTP or duplicate derivative. |
| Native buffered cancellation could race worker shutdown and cause terminal failure. | Parent worker cancellation takes precedence after published-receipt reconciliation. Recovery test retains both reservations. |
| Native deadline needed to cover foreground work too. | Process watchdog stops the isolated service, and foreground launch requires remaining time; original caps never replenished. |
| Doctor fault fixture could fall through to native public schema. | Separate storage/legacy/native databases; v10 intended fault tests pass. Failed v6 evidence retained. |

Final reviewed code had no known unresolved material P1/P2 finding. Safe failure
diagnostics store only the allowlisted transport code and numeric HTTP status;
raw Message/ProviderCode/Details are excluded and regression-tested.

Independently counted `validation-v10`: **854 PASS, 0 FAIL, 2 SKIP**;
`diagnostics-v11`: **239 PASS, 0 FAIL, 0 SKIP**. Runs overlap and cannot be summed.
Both final chains completed. Skips remain skips, not passing evidence.

Subsequently reviewed test-only `f5939e7` and the original `crash-v14` log:
**5 PASS, 0 FAIL, 0 SKIP** (four cases plus parent). Complete HTTP request bytes
are received before response; Kill/Wait explicitly confirm `signal: killed` on
Linux. Publication crash also confirms signal exit. SQL asserts two distinct
reservations, one unknown usage, one derivative and three charged scopes with
two turns each. Source SHA256 matches. The earlier v13 lacked the Kill/Wait
assertion and remains weaker historical wire/recovery evidence. No production
code changed, no cloud inference was added, and no new material finding remains.

Independently read native-v2 receipts/final-audit and queried the isolated PG:
two admitted attempts, two linked reservations, zero known usage, zero published
derivatives; owner/day used 2/3 turns, 2,113,536/3,170,304 reserved tokens,
180,000/270,000ms and 16,777,216/25,165,824 input bytes. Control disabled;
isolated Orchestrator exited. Native windows have no successful `complete` marker.
Native-v1 exact cause is unknown; native-v2 returned `provider_unavailable_http_403`.
These are conservative charged attempts, not proof of two upstream executions.

**Full #82 acceptance is BLOCKED / NOT COMPLETE.** The configured Antigravity
`gemini-3.8-flash-tiered` rejected the native inference request with HTTP 403.
The safe evidence does not establish the cause of that access/policy rejection;
active/connected and successful auth resolution do not explain it.

Not proved natively: successful provider response, validated quote, atomic
publication with observed usage; publication/restart/lost-ACK recovery; completed
foreground run while optional local-only work waits; model quality. Their real-PG
controlled-provider and actual-adapter/HTTP tests are recorded separately.
Draft PR is appropriate; neither native promotion nor automatic issue closure
is supported by these results.
