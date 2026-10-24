# Credential vault implementation and acceptance

Owner decisions: separate login/password, optional site hints without static
domain restrictions, protected UI with a right-center bookmark, six-cell PIN
and sidebar, two credential cards (access existing / create), protected OTP
input, no chat commands or legacy credential import. PostgreSQL remains durable
truth; Tool Broker mediates every model use. Android automatic OTP relay is a
separate rollout from the manual OTP baseline.

## Delivery stages

1. Encrypted entry fields, owner-scoped alias/login search, versions and revocation.
2. Server PIN/unlock lifecycle and protected owner API; no secret logging.
3. Web bookmark, six-cell first/repeat PIN, sidebar CRUD, access/create cards.
4. Fixed Broker contracts, current browser attestation, durable interaction wait
   and resume on the same owning Input; origin/field/version checks at release.
5. Private Chromium comparison (`credential_verified`) after input; no values,
   hashes or lengths in observations. Form submission and authentication remain
   separate evidence states, including the existing unknown-effect stop.
6. Manual OTP, short TTL, exact-call reservation and protected owner response.
7. Telegram control-card delivery with a protected HTTPS Web link, current bot
   binding/export policy, durable send/edit claims and no uncertain replay.
8. Automatic Companion OTP: explicit source/device permission, active challenge
   correlation, transient private relay before ordinary notification masking,
   ambiguity/manual fallback, and real-device acceptance. Platform-hidden OTPs
   must not be recovered or promised.

## Required acceptance

- PostgreSQL: same login/different passwords; independent fields; no plaintext
  columns; owner isolation; version CAS; revocation; PIN throttling and locking;
  approved destination changes; changed-origin/run denials; single-call OTP.
- API: owner proof, unlock proof, bounded strict JSON, fixed errors/no-store,
  metadata-only responses, no event/transcript ingestion of secret input.
- Web Chromium: first/repeated PIN, six cells/paste, CRUD, request types, locking,
  no local/session-storage secrets, no secret-entry screenshots or traces.
- Runtime Chromium: actual input comparison, current-page guards, reflection
  masking, frozen preparation, cookie/storage containment and unknown outcomes.
- Kernel: wait/resume, early and late answers, restart, task changes, expiry,
  input-claim replacement, explicit denials, no permission inferred from search.
- Telegram: current binding/bot, fixed card, protected URL, close projection,
  no raw secret message/callback, durable claim and unknown-send no-replay.
- Companion relay requires its own opt-in and device tests; manual entry remains
  available when notifications are unavailable, delayed or redacted by Android.

## Local validation scope

Work is performed on the Windows development PC. Portable PostgreSQL and pinned
Playwright Chromium are cached in the repository. No shared server, production
database, provider account or real external-site login is part of these tests.
Existing Linux-specific native-release fixture failures must be reported
separately and never represented as passed subsystem acceptance.
