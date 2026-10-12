# Separate final High window preparation

This change prepares a separate, explicit two-attempt fixture with baseline 39
and ceiling 41. It does not execute inference, reopen a prior one-use marker,
increase the prior window's ceiling 39, refund its calls or rewrite its rows.
The root task owns the later single execution and its distinct marker after
integrated source validation and the connection-only preflight.

The selector is `BUTLER_DELEGATION_HIGH_FINAL_WINDOW=1`. Final limiter dispatch
precedes remainder/full High dispatch, and the selector retains Flash High model
routing. The existing 37/39 limiter, its history reader and expiry logic remain
unchanged. Scenario, mixed provider and runner composition are integrated by the
parent task separately; this helper does not claim those paths were exercised
by its component tests.

API: `highFinalDelegationWindow`, `highFinalDelegationHistoryPaths`,
`readHighFinalDelegationHistory(paths)`, `newHighFinalDelegationLimiter(ctx,pool)`
and `(*nativeDelegationLimiter).reserveHighFinal(ctx,kind,runID,input)`.
Every start/continue/tool-result entry revalidates all thirteen fixed pins.
The thirteenth is `deploy/delegation/native-remainder-high-20261012.json`, SHA-256
`58edb6f022399e385118c51de8e3f9f7890fbfc74a5dd3b39de8b42839651505`,
release `bdd85cdc752a739e0a4fa9be4789bf2f8fc7132d`. The new reader uses the
existing remainder reader's 64 KiB bound; older reader bounds stay intact.

The exact failed report carries two native starts, ordinals 38–39, and eight
scripted turns recorded separately. Width1 is a scripted component baseline;
width2's scripted parent receipt passed but native child finals/objective/overlap
and subsequent owner controls did not pass. For each native attempt the reader
matches its run hash to exactly one owned failed run with `transport_error` /
`transport_connection_error`, requires NULL input/output usage and rejects a
native final row. It does not convert this failed scope into completed acceptance.

The historical chain retains 39 usage entries: 17 known pairs and 22 unknown
pairs. Known totals stay 62,760 input tokens and 1,810 output tokens. The report
adds two spent attempts and extends the baseline hash. Unknown usage remains
NULL and is never zero or newly available budget.

PostgreSQL admits at most two new slots, ordinals 40–41, with an immutable
18-minute deadline. A counter-row lock plus fresh database-clock check denies
post-wait expiry before committing either write. Reconstruction preserves the
counter and original deadline. The constructor checks an existing table's own
limits/hash before DDL, so an old spent 37/39 row cannot be upgraded merely
because its counter equals 39. Both its row and trigger definitions remain intact.
Production owner/root/day budget admission and policy remain independent.

## Retained connection diagnosis

The failed report SHA above matches the lab proof. Source hashes for the actual
router, limiter, provider and runner matched that proof's manifest. Both committed
attempts matched owned run hashes with `transport_connection_error`; in the
Antigravity protocol this class is emitted by an HTTP-client error after request
construction with no context cancellation. Router/fixture denials use a separate
fixed error, authentication failures use `provider_unavailable`, and upstream
HTTP rejections have their own classes. This establishes failure beyond local
admission; it does not establish which network hop failed or an upstream quota
rejection. The original transport error's detailed subtype was not retained.

Read-only proxy metadata showed the existing Xray container running with zero
restarts and no OOM, and its backend alias present. Two concurrent public
`example.com` HEAD requests through it returned HTTP 200. Two concurrent
CONNECT/TLS-only lanes to the actual daily Cloud Code hostname, and a separate
pair to the requested sandbox hostname, each reached TLS with CONNECT 200.
No HTTP request followed TLS, and no authentication, account, metadata or
generation request was sent. This later connection health does not prove the
older HTTP requests succeeded or identify a transient failure's cause. No proxy
or limiter bug was demonstrated, and no production change was made for it.

The safe TLS proof is retained by the root task at
`deploy/antigravity/high-parallel-tls-only-20261012.jsonl`, SHA-256
`c9ac37cb765d4e572fbc0fed19cc4294a2cb76ab46bc43444f0e441a0ab6caaa`.
The original read-only report is
`/srv/storage/butler/agents/proxy-connectivity-diagnosis/reports/tls-only-20261012T060718Z-212296/status.jsonl`.
These probes sent zero model calls; native accounting remains 39.

## Isolated tests

All helper validation uses SSH `laba`, Docker Go 1.27.2 and PostgreSQL 17 with
pgvector on a disposable internal network. No imported credentials, native
authorization flag or provider call was used. Vet runs with network disabled.

Five top-level tests cover historical failure scopes, 13-pin missing/forged
revalidation on all three methods, sixteen concurrent contenders for exactly
two slots, immutable metadata/reconstruction, the spent old 37/39 window's row
and trigger preservation, and actual unchanged-row wait/natural expiry/rollback
with counter 39 and no attempt row. Ten additional pure scope cases reject
promoted failure, forged counts, scripted/native confusion, NULL-to-zero usage,
duplicate runs, fabricated native finals and changed transport/control outcomes.

The exact failed artifact is copied for validation and remains parent-owned.
Only the new helper/document and minimal selector/limiter dispatch edits are
committed by this task.

Final narrow evidence:

- Agent root: `/srv/storage/butler/agents/delegation-high-final-window`.
- Tested snapshot: `20261012-131626-141`.
- Report: `reports/expiry-green-20261012T061703Z-244579/tests.jsonl`.
- SHA-256: `e0298fe4e51e29da3257a0b04d6e0ec3ea9da507802a43721b984b0498ba56e8`.
- `go test -race`: 15 passing tests/subtests, five top-level, zero failures/skips.
- `go vet ./apps/orchestrator/internal/app`: passed with network disabled.
- Filter: `TestHighFinalDelegation` followed by History, LimitIntegration,
  RevalidationIntegration, OldWindowDeniedIntegration or
  ExpiryAfterUnchangedRowWaitIntegration.

The initial implementation also passed the same five tests and ten scope cases,
report `reports/expiry-green-20261012T061504Z-237072/tests.jsonl`, SHA-256
`bf5300c71ea527a6d5e76739f8d93118c68df0d48de78d8f9d8b61c307ca85bf`.
The final source additionally tests a fully spent old counter 39 with its two
ledger rows, then proves the denied new constructor/admission preserves them.
