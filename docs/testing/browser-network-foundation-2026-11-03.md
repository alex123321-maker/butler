# Browser network foundation — 2026-11-03

Scope: the first prerequisite for #165, based on main
`415a7cc8326f29922b3d6ccfd4e9d258e0e7e243`. This is not full SPA acceptance.
No lab deployment, model attempt, credential request or native delivery was
performed for these checks. Historical #164 timings were not rerun or changed.

## Local evidence (Windows, race detector, actual Chromium)

| Check | Result | Recorded interval, Asia/Tomsk |
| --- | --- | --- |
| Entire egress package | 16 test passes, 0 failures, 0 skips | 09:19:07.462–09:19:10.505 |
| Actual runtime gRPC/helper/Chromium session suite | 11 passes including parent and 10 subcases, 0 failures, 0 skips | 09:18:18.697–09:18:28.263 |
| Vet: egress and browser runtime | PASS | separate static check |
| Changed-file/spec mapping and whitespace check | PASS | separate static check |

The six new proxy tests check immediate SSE chunk delivery, cancellation on
freeze, cumulative HTTP inbound limits, the shared HTTP/CONNECT allowance,
terminal outbound overflow, pre-DNS concurrent admission and finite lifetime
admissions across freeze/thaw. The actual Chromium test observes a genuine
EventSource event, calls `browser.prepare_fill`, independently observes recipient
disconnect, then verifies the frozen observation and absence of reconnects over
four fixture retry intervals. Existing negative POST/WS/private-origin,
credentials, form continuity and crash/expiry cases still pass.

The first aggregate-overflow fixture initially failed because the fixture's Go
client transparently retried a GET on a reused connection and received a clean
403 after the proxy had exhausted. The fixture now disables keep-alive so the
original aborted response is asserted directly. This was a test-client correction;
no mutation retry was introduced. Final results above follow the correction.

These test intervals are process/test duration, not agent task speed. There was
no native model run here, so model deliberation/native completion latency are
**not measured**, not zero. The dedicated CI script rejects required skips and
preserves JSONL reports. CI result and reviewed commit belong in the PR/issue.

## Remaining issue gates

The [network contract](../architecture/browser-network-contract.md) distinguishes
implemented controls from targets. POST-query authority, WS/WSS messages,
HTTPS streaming qualification, continuous stop/revoke/owner fencing, worker
coverage/cache/update/restart, hostile reconnect/stream cases and the native
model-to-delivery SPA scenario remain open. Raw CONNECT fixture coverage is not
an actual TLS stream or worker proof. Existing SSE support does not establish
the future POST/WS streaming contract or independent remote effect confirmation.
