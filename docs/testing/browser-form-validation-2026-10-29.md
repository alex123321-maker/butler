# Native public form validation qualification, 2026-10-29

The new `form-validation-01` Input completed the public form task with explicit
owner assistance: observe invalid quantity 8, correct it to 3, submit once, read
the Delivery reference and deliver the privately verified answer. Input/run
completed, Web became result_ready and Telegram acknowledged sent. The joined
[qualification](browser-baseline/form-validation/form-validation-01-qualification.json)
is **assisted_complete**, not autonomous reconciliation. #163 remains open.

## Implementation and privacy boundary

Browser observation now carries bounded public form constraints and local
validity flags. Quantity 8 on the fixture's min1/max5 field yields
`range_overflow`; correction to 3 yields valid. The observer reads ValidityState
without invoking checkValidity, reportValidity, submit or native invalid events.
Validity is returned only with network routes frozen. No field values, patterns
or native error messages are added. These observations do not confirm POSTs.

Password, hidden, file or unknown controls suppress feedback for the entire form.
Known cookie/storage/credential state suppresses feedback for the context. Go
independently bounds and validates metadata and keeps suppression sticky after
Broker-resolved credentials, even after the field is overwritten with plaintext.
Secret collection before and after capture suppresses late discoveries. This is
a conservative boundary, not an assertion that arbitrary page script is trusted.
Input tool schemas, revisions, hashes, effect verifiers and approval policy stay
unchanged. Existing optional form output data accommodates the observations.

## Actual Chromium and PostgreSQL checks

All selected cases passed under race without required skips, using the actual
narrow browser source/binary and isolated PostgreSQL plus real Broker/Chromium:

- Local invalid8/corrected3 observations caused zero POSTs and zero invalid events.
- Password, hidden, cookie, storage, late-cookie and credential-then-plaintext
  cases suppressed metadata and retained credential canaries outside output.
- Broker integration submitted exactly one corrected quantity3 POST, preserved
  effect_unknown and refused a spent-grant replay without another POST.
- Existing browser session, GET form, Broker sequence and actual runtime-loss
  recovery regression parents passed. Metadata privacy/bounds units and vet passed.

Evidence: [build/race/vet](browser-baseline/form-validation/build-20261029T095213Z-2398007/),
[Broker and Chromium](browser-baseline/form-validation/chromium-20261029T095307Z-2402688/),
[additional privacy cases](browser-baseline/form-validation/privacy-20261029T100028Z-2435076/).
Both new Chromium parents are mandatory no-skip cases in credential-vault CI.
PR checks for this source SHA are reported separately; earlier green CI is not
evidence for this new revision.

The first new CI SHA `fcfb31cd` failed the credential-then-plaintext runtime test
after the Broker cases passed. Content-free diagnostics and full-source local
repetition reproduced stale_observation before the next input: **11/20 passed**,
then **16/20 passed** after adding a fresh snapshot. The guard was not weakened;
the exact late page-event source is not proven. Calling FailNow through a parent
testing.T also obscured the subtest failure; helpers now use the current subtest
and print only closed error codes, never response material.

The username privacy fixture now uses an ordinary text field, explicit fresh
snapshots, one plaintext replacement and checks before/after both observations.
All fields still must suppress metadata, and the numeric public invalid8/valid3
and private-number unit cases remain. **20/20 full-source Chromium repetitions
passed** under race. This changes the privacy test fixture, not production input
timing or the stale guard; it does not prove that arbitrary numeric credential
input is immune to late page revisions. Failed series are retained alongside the
successful series under `full-repeat-*`; native Inputs were not repeated.

## Native task and independent receiver

Canonical owner ingress used the configured Antigravity/gemini-3.8-flash-high
provider. The prompt specified the task and quantities, not tool calls, selectors
or the expected reference. No old Input or baseline slot was rerun.

The first title preparation safely failed on a stale observation after the
fixture's delayed help mutation. The model refreshed its snapshot and continued.
Invalid8 was recorded frozen at 10:01:26.581854 UTC; corrected3 valid at
10:01:37.537623 UTC. The independently fsynced receiver journal recorded the only
POST at 10:02:00.439747983 UTC, accepted quantity3; there were zero invalid POSTs.

Submit remained UNKNOWN in its original transcript. An explicit protected
factual owner report, based on that receiver journal, confirmed the effect and
released the existing run for the next read; there was no replay. Assistance was
**five exact public-action approvals plus one factual operator report**. No
credential request, PIN/budget reset, synthetic machine receipt or fault injection
was used in this trial. Every durable action effect has exactly one dispatch.

The model then read the reference through browser.read and closed its context.
One publication draft was rejected; an additional web.fetch read preceded the
accepted answer. This is actual browser coverage supplemented by one HTTP read.
Private comparisons confirm the reference was present in both observations and
in the delivered answer; validation/correction and the title/quantity were also
reported. Raw answer, fixture config and control reference are not exported.

Joined evidence: [local validation](browser-baseline/form-validation/form-validation-01-local-validation-audit.json),
[receiver](browser-baseline/form-validation/form-validation-01-recipient-audit.json),
[durable audit](browser-baseline/form-validation/form-validation-01-final-audit.json),
[owner report](browser-baseline/form-validation/form-validation-01-owner-outcome.json),
[private reference comparison](browser-baseline/form-validation/form-validation-01-read-verification.json),
[answer validation check](browser-baseline/form-validation/form-validation-01-validation-answer-check.json),
[lifecycle and delivery](browser-baseline/form-validation/form-validation-01-metadata-final.json).
The form verifier's expected_answer_present=false is its read-only-specific
control; the independent read comparison is true. Form recipient/answer criteria
are independently true. Neither verifier alone asserts complete-task success.

## Inclusive timing

Input acceptance 09:58:46.382277 UTC through completion 10:04:55.014550 UTC took
**368.632s**. Web was available at **358.693s**, Telegram sent ACK at **360.216s**.
Delivery overlaps execution and is not a human-read receipt.

All **19 tool spans** are retained, including the stale preparation failure,
UNKNOWN submit and rejected draft. Sequential action intervals include model
deliberation/context, scheduling, approval/operator waits and tool execution.
Their total is 358.801s: 355.867s of intervening decision/context/wait intervals
and 2.934s of tool spans. Finalization is 9.831s; the complete partition sums to
368.632s. These gaps cannot be separated into pure model-only latency.
There were 19 model attempts, **230,597 known tokens**, zero unknown usage pairs;
monetary cost is unavailable.

[Raw timing](browser-baseline/form-validation/timings/form-validation-01.json),
[every action and model accounting span](browser-baseline/form-validation/inclusive-actions/README.md).
The separate operator window from first timestamp 09:37:09 UTC to joined
verification 10:09:52 UTC was **1,963s (32m43s)**, including Codex deliberation,
implementation, tests, rollout, waits and verification. Documentation and CI after
that boundary are separate. Time before the first timestamp is unavailable.

## Deployment and limits

Only the browser was narrowly rolled out, 09:55:26.951845910–09:55:33.162281026 UTC:
binary SHA256 `2b4634aabbc75dbe17c2961e18d834dc57144a2604a5e492dad75d1bbb7bfad4`,
release `/srv/storage/butler/main/releases/browser-form-validation-20261029-v15`.
Core v14, Broker v13, PostgreSQL, Redis and Web container identities/start times
were unchanged. Same image/command/environment and read-only release mounts;
health passed, zero migrations, no credential/budget reset. This is not a full PR
deployment or a new owner-review UI rollout. [Rollout record](browser-baseline/form-validation/rollout-v15.json).

All historical baseline and seven native-runtime-loss trials retain their
outcomes. General browser acceptance stays **NO_GO**. This additional assisted
form result does not establish autonomous POST receipts, arbitrary mid-POST
recovery, popup SSO, CAPTCHA or WebSocket support. The preceding actual runtime-loss
and native login qualifications remain separately bounded in their reports.
