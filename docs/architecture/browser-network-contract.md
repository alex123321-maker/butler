# Browser network: implemented foundation and remaining SPA work

Issue [#165](https://github.com/alex123321-maker/butler/issues/165) extends the
browser network, not model-provider transport. Tool Broker continues to own
authority, credentials and effects; Go owns the private egress proxy; the
reviewed Playwright helper executes the bounded browser operation. This document
separates the first implemented foundation from the remaining target.

## Implemented foundation

Every browser context retains its existing finite expiry and outbound allowance.
Its private proxy now additionally enforces:

- at most 64 active HTTP requests / CONNECT tunnels, checked before DNS or dial;
- at most 2048 admitted HTTP requests / CONNECT attempts over the context's
  entire lifetime, including attempts subsequently rejected by destination policy;
- one shared 20 MiB inbound allowance for HTTP response bodies and raw CONNECT
  bytes. CONNECT accounting includes TLS records; cleartext HTTP accounting is
  body bytes, not an exact wire-volume measurement.

Completing a request releases concurrency only. Freeze, observation, owner return
and a new authorized operation do not reset lifetime attempt or byte counters.
Overflow of either byte direction or the lifetime attempt ceiling freezes the
context terminally: `AllowUntil` cannot reopen it. Ordinary concurrent-capacity
denial is temporary. Limits remain code-owned, not model arguments or permission.
These local inbound/admission ceilings are separate from PostgreSQL's existing
outbound reservation; this slice adds no inbound budget/refund API.

The cleartext HTTP proxy flushes `text/event-stream` chunks as they arrive, using
the same cumulative inbound counter. Cancellation, truncation and overflow abort
the response instead of manufacturing a clean successful EOF. This is a proxy
capability, not a new streaming model tool. Actual Chromium EventSource through
the existing GET-only helper path receives an event, then `prepare_fill` closes
the recipient connection before input; automatic reconnect cannot cross the
persisting freeze. CONNECT closes on the same freeze and shares the byte cap.

DNS/IP pinning, operator domains, HTTPS hostname verification, expiry, helper
GET/HEAD restrictions, page ownership, protected login and UNKNOWN/no-replay
remain independent. HTTP fixture resolver/dial seams exist only in tests.
No new dependency, public port, operator flag or database migration is added.

## Remaining target; not enabled by the foundation

1. **POST/GraphQL loading.** An intent to read does not make a POST safe. Keep old
   read/prepare contracts strict. A new interaction profile must pin the actual
   permitted destinations, methods, payload limits and authority window under
   Broker, account for background effects, retain unknown outcomes and never
   reuse a consumed mutation after lost response. A fixture query plus filter
   change must traverse production Core/Broker/runtime with recipient evidence.
2. **WS/WSS and bounded streaming.** WebSockets stay denied. A future private
   connection capability must check the actual URL/origin, public DNS/IP, TLS,
   redirects and each released message, plus connection/byte/time/reconnect
   bounds. An opaque CONNECT is not sufficient application-level authority.
   Existing and new connections must share stop/revoke/preparation/owner fences;
   buffered messages cannot flush after a later thaw under old authority.
3. **Current authority during long connections.** Proxy expiry/freeze is not a
   new online revocation subscription. Prove propagation from durable stop,
   mandate revocation and owner takeover through the production composition to
   every active stream, with independent recipient observations after ACK.
4. **Service workers.** They stay blocked. Before enabling, prove interception
   of actual worker-originated requests, redirects, cache/update/restart and
   worker lifetime with the same fences. Ordinary page interception is not
   evidence of worker coverage; an uncontrolled worker path blocks this slice.
5. **Qualification and diagnostics.** Test private/metadata destinations, mixed
   DNS/rebinding, redirects, foreign origins, oversized/infinite streams,
   reconnect storms and runtime loss. Bound errors without URLs, payloads,
   cookies, authorization headers or credential material. No fixture-only
   domain exemption can enter production.

The separate native scenario must use the live model and ordinary owner channel,
not a direct helper call. Timing starts at accepted ingress and includes model
deliberation, tool discovery, scheduling, owner waits and final delivery. Tool
duration, engineering/test time and native end-to-end time are separate metrics.
Do not replay old scenarios to reconstruct missing historical timing.

## Required checks for this foundation

`bash scripts/dev/browser-network-tests.sh` runs the proxy suite under the race
detector, vet and actual Chromium session regressions. Required cases cannot
skip. Dedicated CI preserves value-free JSONL reports. The new tests cover live
SSE delivery/cancellation, cumulative HTTP overflow, a mixed HTTP/CONNECT shared
allowance, capacity before DNS, lifetime allowance across freeze/thaw and actual
Chromium EventSource preparation/reconnect. Existing Chromium tests retain
credential masking, form continuity, forbidden POST/WS, private origins, stale
observation, freeze, expiry and helper-loss controls.

This foundation alone does not satisfy #165's native SPA, worker or full
authority-revocation acceptance. The issue remains open until those checks pass.
