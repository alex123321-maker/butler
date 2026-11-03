# SPA profile qualification, 2026-11-03

Issue165 continuation on main `e22ade73`. Source/acceptance under qualification;
this document does not yet assert full native or deployed acceptance.

## Evidence already collected

- All five workflows on the foundation merge `e22ade736c909e4f844eb17ecb48bc1531393f2b`
  passed: vault41567270559, network41567270385, pages41567270503,
  SSO41567270471 and communication41567270405.
- Real PostgreSQL on an isolated, loopback-only, tmpfs lab instance: all11
  Broker/resource cases passed (allowed, path deny, foreign origin, stop, revoke,
  constraints, source removal, Input replacement/expiry, lost Finish and expiry).
  Linux direct-PG matrix2.50s; positive case0.23s. A Windows/SSH transport debug
  run stalled and is not counted as passing. No production DB state was changed
  for this mechanical matrix. Final source is subject to mandatory Linux CI.
- Actual Chromium proved GraphQL POST+filter, typed WSS subscription, SSE/fetch,
  worker registration/update/cache, and no raw cookie reflection. Worker restart
  has a distinct in-worker boot ID; after terminal closure the browser still
  reads its local cache but cannot update a worker or send a queued request.
- Go proxy race cases cover TLS verification separate from local SPKI trust,
  private/metadata/foreign redirects, DNS rebinding, traffic/compressed expansion,
  infinite stream expiry, current-authority loss, lost query response and no retry.
- Actual runtime fault cases include cancellation, revocation and helper loss
  after the recipient's first accepted query. They retain UNKNOWN/spending and
  do not produce a success proof or a duplicate accepted query.

One broad local command had missing legacy harness environment and a timer-based
legacy stale-observation failure under concurrent load. Re-running the correctly
configured, targeted Chromium regressions passed (32.066s). That failed command
is retained as a failed engineering check, not hidden as native acceptance.

## Required completion evidence

Mandatory CI must pass without skipped required cases. Still record controlled
deployment hashes/rollback, actual owner UI stop through the production kernel,
native live model discovery→read/filter→verified independent recipient→actual
owner delivery, and current production defaults. No fixture egress exemption is
permitted. Close the acceptance checklist only after those results exist.

Mechanical tests make zero model calls. Their durations measure the test,
not agent speed. Native timings start at accepted ingress and end at actual
delivery; model deliberation, discovery, queues, errors, owner waits and delivery
remain included. Historical scenarios have not been replayed for old metrics.
