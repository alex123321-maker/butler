# Scoped one-consent login — 2026-10-30

Implements the ordinary-login follow-up to #163 in draft PR178, stacked after
PR177. The bounded ordinary-login native scenario passed with one consent and
real Web/Telegram delivery. General browser acceptance remains NO_GO; unsupported
forms/SSO/OTP and lost outcomes do not borrow that success. Eight earlier attempts,
all spending and their original transcripts remain in the record.

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

- Real Chromium plus guarded Go egress: eleven cases PASS. Verified login makes
  one POST and two authenticated account reads (browser + independent probe).
  Public landing, mutated formdata/CSRF, wrong password, foreign target, extra
  password/control, hidden business fields, ambiguous forms and stale observation
  cannot confirm authentication. Indexed observed selectors select the intended
  second form without releasing the first form's trap. No
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
test waits. Native timing separately includes model deliberation/context,
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
deployment. The later qualified trial and restoration are recorded below.

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
| 06 | Approved consent; waiting/active recovery race, later terminalized | 705.413s | 9 |
| 07 | Claimed login, unsupported observed selector; original UNKNOWN retained | 227.230s | 9 |
| 08 | Retained trial07 uncertainty stopped the next request before dispatch | 153.291s | 7 |
| 09 | Verified single login and real delivery | 245.569s | 10 |

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

Trial06 reached a correct pending `login` request after the source-observation
fix and received the owner's explicit approval. Recovery then hit a second
ordering defect: the active-only Task supervisor ran while the canonical
interaction transaction still had the Task in `waiting_for_user`. The Input
was blocked at `interaction_ready`, with zero login dispatches and an unclaimed
grant. It was stopped and its authority revoked through owner APIs; the consent,
failure, original expiry and all spending remain in history.

The recovery follow-up resolves the canonical interaction before starting the
active-only supervisor, with resolution bounded by the newly acquired lease's
expiry and its existing claim/version/evidence checks. A400ms delayed real-PG
resolver reproduces the old failure; the changed code passes all seven cases,
including unknown, expiry, cancellation, changed constraints and source denial.
No provider request is admitted on the negative cases. The compatible Core's
previous binary was reproduced byte-for-byte before applying only this change;
its delayed PG/race, credential wait and watchdog tests and build pass. Initial
backport compile/reproduction failures remain in the build timing ledger.

Trial07 resumed the approved interaction correctly and released both fields to
one claimed login call. It then failed with UNKNOWN and zero recipient POSTs:
the observed `form >> nth=0` selector was incompatible with the helper's native
CSS query. No call or grant was replayed. Selector compatibility now accepts the
bounded indexed form produced by observations and requires unique unindexed CSS.
Eleven real Chromium cases pass, including a positive selection of the second
form and a negative ambiguous-form case; the original exact-body/no-extra-traffic
and independent access-probe checks remain. The initial container test harness
could not execute its RAM-mounted binary; its failure is retained separately.

CI on469fe55f exposed a real cancellation regression in the first recovery-order
patch: delaying the whole supervisor also delayed lease-failure cancellation
during answer recovery. The correction keeps lease observation/renewal active
throughout and gates only active-only Task checks until canonical resolution.
The existing three-phase communication cancellation regression and all seven
delayed interaction cases pass together on Linux PG/race. The earlier failed CI
remains evidence, not a flaky result or a pass.

Trial08 was blocked at credentials.request_login by the retained trial07 UNKNOWN;
no new vault request or POST occurred. The session-wide stop fence was preserved.
The old recipient was stopped at06:35:38 UTC, with zero journal events and the
SHA256 of an empty file; its original browser context expired at06:25:14.657 UTC.
A historical-only lost-login report path now requires that original expiry plus
the35s maximum runtime window, a terminal dispatch/run and no live lease, exact
typed login policy/args provenance and a factual authenticated recipient/settled
report. Real PG/race tests cover twelve positive/negative cases and unchanged
generic owner-outcome boundaries; Windows targeted PG tests also pass. Such a
historical report remains owner-assisted maintenance and cannot qualify native
login, renew a grant or resume the old Input.

## Qualified native result

Trial09 used ordinary protected Web ingress, a finite owner-goal/mandate and the
configured native provider. Input `input-2d149d6a3204893e67868935e418e0a2` and run
`run-1793342400409497517-8ed43b1ca3944768` completed. Joined evidence proves:

- One explicit vault login consent, one claimed call and one confirmed login
  dispatch; zero generic clicks, extra action approvals or factual reports in
  this Input, and zero rejected answer drafts.
- The sealed recipient journal has exactly one accepted login and two
  authenticated account reads. The runtime independently observed the anonymous
  challenge and PG confirmed the pinned authentication proof.
- The fresh private delivery reference was observed through browser.login and
  matched the protected published answer, with no web.fetch fallback.
- One result publication became ready in Web and received a Telegram API ACK.
  This proves channel delivery/availability, not a human-read receipt.
- Username/password canaries were absent from eight tool-call args/results,
  three transcript messages and the protected final answer. Provider wire and
  system logs were not exhaustively scanned; no broader secrecy claim is made.

The [joined qualification](browser-scoped-login/evidence/native-scoped-login-09-qualification.json),
[durable authority audit](browser-scoped-login/evidence/native-scoped-login-09-authority-final.json),
[recipient/publication comparison](browser-scoped-login/evidence/native-scoped-login-09-login-final-verification.json),
[canary audit](browser-scoped-login/evidence/native-scoped-login-09-transcript-canary.json)
and [sealed recipient counts](browser-scoped-login/evidence/native-scoped-login-09-recipient-sealed.json)
are content-free evidence. Formal Task-criteria verification/goal completion was
not claimed. The test Task was stopped and its goal/mandate revoked after the
completed Input and real delivery were independently checked.

Historical maintenance for trial07 used one separate authenticated factual
operator report against the retired empty recipient journal. Its journal outcome
is now absent, while its original UNKNOWN tool transcript and failed Input remain.
The ordinary stopped-attempt sweeper then classified trials07/08 as known without
deleting history. That report is outside trial09 and is not machine login proof.
Across this qualification phase, the owner approved trials06,07,09; only the last
one qualified. Do not present the successful trial's single consent as the total
human assistance spent on all failed qualification attempts.

## Inclusive timings and teardown

Trial09 took **245.569s** from Input acceptance to terminal completion. Web result
availability was **227.810s**, Telegram ACK **232.459s**. The vault decision wait
was **50.178s**; approval-to-Telegram ACK was **98.794s**. The browser.login Core
tool span was **1.265s**, and browser.open **0.481s**. Tool spans are diagnostics,
not the primary end-to-end agent metric.

| Sequential phase | Elapsed, seconds |
| --- | ---: |
| Acceptance, discovery, account search, page open and consent request | 83.487 |
| Wait for explicit owner decision | 50.178 |
| Recovery, model deliberation and verified login after approval | 70.307 |
| Answer preparation/publication and Telegram ACK | 28.487 |
| Remaining finalization after Telegram ACK | 13.110 |
| Complete Input | 245.569 |

All gaps include model deliberation, context, orchestration and scheduling; they
are not pure model-only measurements. There were ten model reservations,101661
known input tokens and1260 output tokens, with zero unknown usage pairs. No money
price is inferred. [Per-action CSV](browser-scoped-login/summary-final/actions.csv),
[all nine trial intervals](browser-scoped-login/summary-final/trials.csv),
[model accounting spans](browser-scoped-login/summary-final/model-accounting-spans.csv)
and [partition notes](browser-scoped-login/summary-final/README.md) retain all54
recorded tool spans, failures, terminalization tails and unknown values. Trial06's
705.413s includes waiting and delayed failure finalization; it is not eleven
minutes of browser execution. Trial03's terminal duration remains unknown.

The four original lab services were restored and independently verified at
**06:50:49 UTC**: binary/catalog/mount fingerprints and environments match the
initial snapshot; PostgreSQL/Redis were not restarted. The initial mount check
failed only because Docker changed array order; a normalized read-only comparison
passed without a second rollout. Migration106, evidence and spending were retained.
The qualified recipient journal was sealed, a fresh idle fixture installed, and
the isolated test PostgreSQL stopped at06:52:14 UTC. See the
[restoration receipt](browser-scoped-login/evidence/baseline-restored-final.json).
Nothing was merged into main or deployed as a full-main release.

## Final CI qualification

Implementation revision `aa4d4549ead45056e927ad7efa153143949e3920` passed **11/11
jobs across all five workflows**: [discovery](https://github.com/alex123321-maker/butler/actions/runs/41015122399),
[Web](https://github.com/alex123321-maker/butler/actions/runs/41015122456),
[Android Go/build and four emulator versions](https://github.com/alex123321-maker/butler/actions/runs/41015122712),
[real PG/browser vault](https://github.com/alex123321-maker/butler/actions/runs/41015122470),
and [communication plus converter matrices](https://github.com/alex123321-maker/butler/actions/runs/41015122411).
The communication matrix includes the previously failing three-phase recovery
cancellation test. Earlier failed runs remain linked to their original commits.
The remaining evidence-only commit changes no implementation or runtime bytes.

Before publication,33 public report/CSV/JSON files (157256 bytes at that audit
snapshot) were privately scanned for the actual username, password and fresh
private reference; none was present. See the
[artifact audit](browser-scoped-login/evidence/public-evidence-canary-audit.json).
This is a fixed-needle check, not a universal secret detector.
