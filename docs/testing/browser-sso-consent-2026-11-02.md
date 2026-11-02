# RP/IdP consent foundation and post-merge CI

Issue #164 remains open. This stage implements protected consent storage and
presentation, not an installed SSO tool, native execution or Google acceptance.
See [protocol and remaining gates](../architecture/browser-sso-protocol.md).

## Post-merge observation

Browser integration merge `651e3fde8ce3e0f59b7ba3cac1f5ba6a2a507f31` has passing
[discovery](https://github.com/alex123321-maker/butler/actions/runs/41386928873),
[Web](https://github.com/alex123321-maker/butler/actions/runs/41386928839),
[browser SSO fixture](https://github.com/alex123321-maker/butler/actions/runs/41386928967)
and [vault](https://github.com/alex123321-maker/butler/actions/runs/41386928962).
The subsequent `9134fc9e0f5daf87c1553141a749e4c2c9aee20c` adds only the reviewed
Arc diagnostics. Browser/core source is unchanged between those merge trees.

[Communication](https://github.com/alex123321-maker/butler/actions/runs/41386928827)
is **not PASS**: converter passed in 10m06s, communication was cancelled at the
25-minute job limit. Its retained six JSONL reports all end in package PASS and
contain zero fail events. The last private-control report finished at
04:35:14.975 UTC; cancellation followed at 04:35:18.202 UTC during final build
commands. Successful test reports do not prove those builds completed.

The downloaded reports remain at `.cache/main-communication-41386928827/`.
Their last timestamps are targeted 04:25:16.200, local-authority 04:25:33.065,
catalog-source 04:28:47.918, budget-policy 04:28:56.123, regression
04:34:41.884, control-regression 04:35:14.975 UTC. Operator/model inference and
external channel delivery were not invoked.

The fix splits the communication kernel and general regressions into separate
25-minute matrix jobs, alongside converter. Package lists, selectors, race
flags, existing explicit exclusions, skip guards, vet and all build commands
are retained. `--regression` still runs the combined operator profile;
`--regression-only` supplies the separate CI job. Fixed stage names with UTC
timestamps make cold build/test progress visible. Passing the replacement
workflow is required before claiming the timeout resolved.

## Local verification

Windows, repository-local caches, isolated PostgreSQL on loopback port55443,
synthetic encrypted account only. Each integration fixture uses its own schema
and drops it on cleanup. No lab deployment or shared service was modified.

* Final targeted race run: **31 test/subtest PASS, zero FAIL, zero SKIP** across
  `internal/browsersso`, `internal/credentials` and production owner API. Packages
  completed in 5.312s, 11.318s and 2.139s respectively; these times overlap and
  are not added as turn latency. Report `.cache/sso-consent-go-final-ttl.jsonl`.
* Coverage includes exact card/version marker, cross-request digest rejection,
  eight simultaneous repeated answers with one durable decision, owner/task/
  constraint/account/context/page fences, TTL clipping including a PostgreSQL-
  observed real row-lock wait past expiry, privacy, no legacy
  field release, account deletion, immutable history and downgrade refusal.
* Chromium: **6/6** vault UI tests passed, including two new SSO card cases.
  SSO card actions took 5.8s and 1.9s; the complete build/start/test command took
  2.4m. Report `.cache/sso-consent-web.log`. UI transport is mocked in these
  tests; the separate owner API tests use actual Go handlers and PostgreSQL.
  This is not an actual combined UI/Broker/SSO execution proof.
* `npm run typecheck`, targeted Vue ESLint, targeted Go vet and Bash syntax
  check passed. ESLint reports the test TypeScript file ignored by its current
  configuration; executable Chromium coverage remains the evidence for it.
* An earlier broader command included `internal/credentialcontrol` with a
  selector matching no tests; its package-level skip is retained in
  `.cache/sso-consent-go-final.jsonl`. The final required profile excludes that
  empty selection and has no skips. No failed attempts are hidden.

## Inclusive timing

All stage wall time includes Codex deliberation, implementation, failed or
superseded checks, compilation, CI investigation and waits. This is engineering
work on the browser agent; **no Butler native login was run in this stage**.
Mechanical fixtures make zero Butler model calls. Earlier native slots are not
replayed for metrics.

| Stage (UTC) | Inclusive wall time |
| --- | ---: |
| Start 04:15:37 to contract/CI audit checkpoint 04:23:54 | 8m17s |
| Contract, ledger, protected API and initial race checks to 04:32:47 | 8m53s |
| Card tests, stronger per-request binding and CI timeout diagnosis to 04:40:23 | 7m36s |

Later CI/review/merge waiting is recorded in the #164 progress comment with its
end timestamp. No mechanical test duration is substituted for inclusive time.
