# Issue76 prospective whole-window owner-day admission

This is a separate operational admission policy for collection v6. The frozen
stream-v5 tool/fixture protocol remains
`issue76-fixture-stream-protocol.md` SHA256
`3e573f54cf61fe2ddd28066c225e55421b47cd0b4cc17d66e36bca9116a57cb5`.
Scope binding, local UNKNOWN/global stop, candidate, source/authority checks,
independent scorer/reader, criteria and limits are unchanged. No native admission
or hidden v6 authorship is established by this public source document.

## Observed predecessor failure

Closed stream-v5 stopped on23rd of48 arms. Its actual owning owner_day scope
expired at2026-10-26T00:00:00Z during admitted submit101. The durable checkpoint
reason is execution_budget_deadline at model_budget_reserved; Controller takes
the minimum scope deadline and Orchestrator installs that context deadline.
The outer transport reported provider_timeout after19851ms without a response
or usage. This is not proof of an upstream timeout or authority latency. The
failed/unsealed final trace and quiescence remain failures, not a recorder pass.
See `experiments/behavior76/2026-10-25/native-stream-v5/` for immutable evidence.

Inclusive history before v6:6 closed windows,506 admissions,500 measured usage,
3832577 known provider-total tokens,6 unavailable usage observations. Every506
historical JSONB row and every1719 stream-v5 raw file must remain exact. No old
window, task, call or stopped tail is resumed; all old negatives remain in audit.

## Strict prospective check

Before creating a new output directory or constructing native transport, compute
the original four-hour window deadline and owning UTC day end. Reject a window
that crosses that day boundary. Before admitting its durable window, read the
owning PostgreSQL clock and require the same UTC day and full-window fit again.
The declared four-hour start includes these admission checks; it is never moved
forward to gain time. Manifest metadata records both observations and the exact
policy hash, separately from the unchanged fixture/tool protocol hash.

No owner-day/session/task/mandate deadline is extended or replaced. Model calls
stay120s maximum, fixture20m, window4h/768 admissions, business/authority reserve
9+1 and shared authority750ms. The owning budget can still impose a shorter
deadline or deny a call, and any unexpected budget/provider/authority/source,
journal, observer, isolation or quiescence fault remains GLOBAL. No retry,
replacement, deadline increase, scope renewal or refund is introduced.

The independent whole-file reader/scorer and all48-arm/96-opportunity/12-family
denominators remain fixed. After public source verification/freeze, a separate
fresh24-episode control corpus is authored independently. Only literal hashes
are bound afterward. Complete zero-native setup/composition and independent
exact producer/ELF admission precede one new window. Masked judgments must seal
before opening its mapping. Partial collection never establishes a full result.

Before this freeze the private host gained a separate Docker28.3.3 test daemon
for issue21, on its own8GiB data volume and mount namespace. The original4GiB
PostgreSQL filesystem, database/history, Go1.27.2, research pgx copy and authority
settings remain unchanged; all506 rows were rechecked after a graceful local
PostgreSQL restart. The daemon stays idle during native collection. No other
container/build/PG/heavy acceptance test is run concurrently with collection.
This operational addition does not establish the cause of historical750ms
observation latency, promote a candidate or imply any production deployment.
