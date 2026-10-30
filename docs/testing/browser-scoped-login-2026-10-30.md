# Scoped one-consent login — 2026-10-30

Implements the ordinary-login follow-up to #163, stacked after PR177. General
browser acceptance remains NO_GO. Native model/provider/delivery qualification
of this new protocol is pending; earlier native trials and timings are retained.

## Implemented boundary

New `credentials.request_login` consent explicitly covers one exact native form
submission and authenticated-access verification. It pins the saved account,
browser, form/selectors, same-origin POST target, current owner/run/Task/revisions
and expiry. Migration106 durably reserves it to one call. It is not a broader
interpretation of old access/create/OTP consent. All ordinary current task,
mandate, commit-effect, resource, deny, account and Input fences remain required.

`browser.login` privately compiles and submits a bounded ordinary URL-encoded
form. Chromium routing admits one exact native POST and at most five same-origin
GET redirects, blocking extra requests and freezing afterwards. Go independently
checks cookie-backed 200 access against an anonymous 401/403 or login redirect.
The runtime returns only typed proof and masked page data; PG rechecks correlated
normalized evidence before confirmation. Missing proof/lost ACK/runtime stays
UNKNOWN and cannot replay the POST. Public success pages are not proof.

This first protocol excludes OTP, popup SSO, JS-only forms, login URL queries,
cross-origin redirects, parent-domain cookies and rotating cookies during probes.
Generic click and POST policy is unchanged. Owner token/PIN were not weakened.

## Completed checks

- Real Chromium plus guarded Go egress: eight cases PASS. Verified login makes
  one POST and two authenticated account reads (browser + independent probe).
  Public landing, mutated formdata/CSRF, wrong password, foreign target, extra
  password/control and stale observation cannot confirm authentication. No
  background trap traffic or credential/cookie canary reaches public output.
- Actual Broker + encrypted PG vault + Chromium: PASS. One distinct vault
  consent, zero duplicate action approvals/factual reports, one POST, verified
  account observation, masked credentials/cookie and denied consent replay.
  Action ownership/journal is a fixture in this composition; the separate PG
  test below exercises the actual durable journal.
- Real PG journal: verified result confirms; missing proof/runtime loss remain
  UNKNOWN across store restart; a consumed grant cannot replay. PG rejects
  missing typed evidence and a changed Input claim before dispatch.
- Real PG vault: exact scope/account, both fields for one call, changed-target
  denial, replay/expiry denial and separation from legacy preparation consent
  pass. Migration rollback refuses to delete login-consent history.
- Owner Web: typecheck/lint PASS; four Chromium vault tests PASS, including the
  explicit one-login card, disabled account substitution, protected approval and
  unchanged PIN/create/OTP tests. No secret-entry screenshots/traces/video.
- Targeted Go runtime/prompt/research/authorization checks PASS. Verified login
  page data supports the final answer without a redundant public refetch.

The broad Windows Go attempt failed and is **not** acceptance evidence. It
included historical/private fixture failures and a short-deadline PG source
proof test; no historical ledger, frozen review or timing was rewritten to make
that run pass. Linux CI and the reviewed server build remain separate checks.
Initial new UI/runtime test-harness errors were corrected; their failed attempts
are retained in the operator timing log.

## Timing interpretation

The successful Chromium login operation, including isolated browser startup and
the independent probes, took about **0.55 s**. This is a component measurement
with zero model calls. It is **not** the native agent's end-to-end login time.

The operator implementation/validation window began **2026-10-30 03:31:44 UTC**.
Its elapsed wall time includes Codex deliberation, coding, build/debugging and
test waits. A new native Input must separately count model deliberation/context,
tool discovery, owner consent, verification and actual Web/Telegram delivery.
No old scenario was replayed to reconstruct previous metrics.
