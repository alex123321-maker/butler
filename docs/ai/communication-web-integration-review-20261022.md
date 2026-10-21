# Web review and communication integration — 2026-10-22

Reviewed base: PR131, `0aaba1b90c27db588e00db305aa70c06b2087e4f`, based on
main `d307a6c6a38e376bb56665cddb2c293b09538803`. Its eight GitHub checks passed.
The owner's later instruction authorizes Web integration. Production activation
and deployment remain separate; this record does not claim either or a merge.

## Review findings and fixes

- P2: run_error's short message bypassed the credential masking used by expanded
  event data. The summary now uses the same masker; the bearer regression test
  checks the whole emitted row.
- Integration blocker: HTTP metrics instrumentation discarded http.Flusher, so
  real run/publication SSE could return an error despite intercepted UI tests.
  Instrumentation now preserves the underlying streaming capability. A real
  HTTP server test reads a frame before the handler completes.
- The replacement UI itself did not implement #129: it read flattened transcript
  messages and had no canonical report/media/reply feed. The integration adds a
  separately gated Web projection without enabling ordinary startup.

## Implemented integration

One canonical publication fans out to its pinned Web and Telegram destinations.
Web serves the full saved report with all typed blocks and required media;
Telegram retains its immutable native parts. Availability, channel ACK, read and
Task completion remain separate. A current owner/binding/source/artifact check
protects feed, detail, stream reconnect and authenticated media download.
Web replies retain the original publication/session/task, and their owning Input
commit writes immutable private ingress proof. A new Web conversation chooses no
Task from the selected panel. Missing profile readiness blocks new sends; a
legacy startup's explicit404 preserves the old ingress.

SSE sends only cursor invalidation. Loaded pages come from PostgreSQL REST and
are replaced after reconnect/refresh, including redaction and independent
delivery changes. Media uses temporary object URLs after digest/size validation;
owner/source changes remove the corresponding view. No raw HTML/JS/remote media
URLs or executable report blocks are rendered. Cursor reads create no transcript
copies or delivery ACKs. Prior pending history is not upgraded to available.

## Evidence in progress

- Twelve focused Chromium cases passed, including the six new communication
  cases and the journal suite. Actual browser download returned exact fixture
  bytes. XSS-like text remained text; a 20-column Unicode table fit a320px view.
- Fresh typecheck passed after fixing a test-local variable that shadowed the
  browser document. Both linters passed. The first complete run had76 passes,
  2 real-environment skips and3 fixture failures caused by a missing explicit
  legacy-profile response. Its helper now returns404; all5 affected Doctor/voice
  cases passed. A final complete run/CI is still required.
- New PostgreSQL tests passed for canonical full report/file reads, cursor,
  actual authenticated SSE and owner/binding/source/media revocation. The first
  wider run exposed old pending-Web fixture assumptions; availability is now
  explicitly gated by the composed Web capability. A fresh wider run is pending.
- Ten composed Core/Broker Web+Telegram scenarios passed in the first run,
  including final/correction/quiet, receipt recovery and exact Web reply retry
  with immutable private ingress proof. These are synthetic model scenarios.
- The actual App.Run/operator Broker/native-provider lab is separately gated,
  finite and credential-reference-only. It has not yet passed this new scope.

No issue is closed by these partial results. #129 requires the same live fixture
in Web and the real Telegram client, including document open/download. #130 also
retains actual application/early delivery, native quiet/missing behavior, full
usage accounting, compatibility/rollback/drain and the unweakened linked AC gates.
