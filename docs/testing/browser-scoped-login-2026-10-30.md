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

- Real Chromium plus guarded Go egress: nine cases PASS. Verified login makes
  one POST and two authenticated account reads (browser + independent probe).
  Public landing, mutated formdata/CSRF, wrong password, foreign target, extra
  password/control, hidden business fields and stale observation cannot confirm authentication. No
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
  denial, replay/expiry denial, concurrent one-call reservation and separation from legacy preparation consent
  pass. Migration rollback refuses to delete login-consent history.
- Owner Web: typecheck/lint PASS; four Chromium vault tests PASS, including the
  explicit one-login card, disabled account substitution, protected approval and
  unchanged PIN/create/OTP tests. No secret-entry screenshots/traces/video.
  The API also requires the explicit `browser_login_once_v1` consent version;
  an already-open old client cannot grant submission through its generic card.
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

## Native qualification and storage blocker

Code revision `9ce77ad481528d93183df45a77b4a98f565c3606` passed all five CI
workflows: [tool discovery](https://github.com/alex123321-maker/butler/actions/runs/41006872638),
[communication/converter](https://github.com/alex123321-maker/butler/actions/runs/41006872720),
[Web](https://github.com/alex123321-maker/butler/actions/runs/41006872612),
[Android](https://github.com/alex123321-maker/butler/actions/runs/41006872696), and
[credential vault](https://github.com/alex123321-maker/butler/actions/runs/41006872598).
These results precede the source-observation follow-up below.

Four lab services were updated at 05:14:50 UTC through a finite reviewed
composition: a narrow compatible Core backport, Broker, browser runtime and Web.
Environments, owner authentication and PostgreSQL/Redis services were preserved;
additive migration106 and all history remain on rollback. This is not a full-main
deployment. No new native login has been qualified yet.

The following new Inputs remain in the denominator. Elapsed times are existing
Input record intervals, including model deliberation, tool discovery and gaps;
they exclude earlier implementation/setup work and are not isolated browser times.

| Trial | Outcome before login | Input elapsed | Model reservations |
| --- | --- | ---: | ---: |
| 01 | Operator token ceiling below provider worst-case reservation | 27.378s | 0 |
| 02 | Operator active-time ceiling below worst-case reservations | 42.740s | 3 |
| 03 | Operator account-title typo; create request canceled without consent | unknown terminal duration | 11 |
| 04 | `memory_source_check_timeout` after login page snapshot | 90.909s | 8 |
| 05 | Task authority deadline while preparing tools.load | 61.724s | 4 |

All five had zero login POSTs and no vault approval. Trial03 performed one
browser.open read; the coarse `zero_browser_effects` cleanup label on trials03/04
means zero writes, not zero reads. Trials01-05 goals and mandates were revoked
without deleting Input, usage or budget history. An unsupported `cancel` owner
control was rejected before trial03 was stopped through canonical `stop`.

A bounded metadata-only PostgreSQL watcher on trial05 identified COMMIT waiting
on WalSync/WALWrite for about0.97-1.26s and a concurrent authority query waiting on
BufferContent for over0.63s, followed by the750ms authority deadline. The server
also reported approximately268MB root and37MB storage available, with elevated IO
pressure. Query-plan probes alone completed in milliseconds. The watcher was
terminated explicitly; no query bodies, credentials or source content were exported.

The converter-composed source observer had retained COMMIT for ordinary unrelated
token-only observations even though the standalone Catalog path already used
rollback. The follow-up uses rollback only after all current source and lifecycle
checks on the isolated ordinary observation path. Returned Inputs, converter
usages, developer/returned-source roles and trusted compound contexts retain their
existing commits. Durability settings, dispatch/publication transactions and
verification deadlines are unchanged. Native qualification after this change is
still required; this is not proof that all storage-induced timeouts are resolved.

Linux PG/race tests passed for ordinary rollback, fresh lease-revocation denial,
and preserved compound converter COMMIT on the full source snapshot. The narrow
compatible Core backport passed its ordinary rollback/revocation test and build.
The Windows attempt
failed in the existing fixture's directory sync with `Access is denied`, before
the new assertions; it is retained as failed qualification. No Windows pass is
claimed for these integration cases.
