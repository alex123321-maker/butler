# SSO fixture prerequisite for #164 — 2026-10-30

Scope: an operator-only two-origin recipient, mechanical Chromium probe,
required race/Chromium CI profile and a target page-addressing proposal. This is
not production multi-page support, model-driven SSO, Google acceptance or native
delivery evidence. Issue #164 remains open.

## Verified results

Windows Go 1.27.2: `go test -race -count=1 ./cmd/browser-sso-eval -run
'^TestSSO(Private|Origin|Fixture)'` passed (package 2.024 s); `go vet` and
`node --check cmd/browser-sso-eval/popup_probe.mjs` passed. Unit coverage includes
the positive RP flow, 12 foreign/replay/expiry/binding negatives, three journal
failure positions, pending-flow capacity/expiry, private initialization refusing
overwrite and invalid config/topology.

Actual Linux Chromium ran in the existing `butler-lab-tool-browser` image
`sha256:3a2e719f086e2d2a110a79f817f235518350aaf87ea47cb51a9a75158e32d9c6`
on `laba`, using only a task-owned test binary under
`/dev/shm/butler-pages-fixture-20261030`. The container had `--network none`,
read-only root, no container log driver, 1 GiB/2 CPU/256 PID limits and an
executable temporary filesystem. No Butler service, policy, credentials,
deployment or historical recipient was changed.

The final non-race Linux binary ran all package tests with Chromium enabled:
all passed, zero skipped. Five real browser subcases:

| Case | Wall time | RP evidence |
| --- | ---: | --- |
| normal | 655.469 ms | 1 provider login, 1 callback, 1 private read; anonymous 401 |
| delayed popup | 800.939 ms | same positive receipts and anonymous control |
| equal-title pages | 609.234 ms | distinct Page objects; 0 callbacks/private reads |
| close before login | 573.730 ms | anonymous 401; 0 callbacks/private reads |
| forged completion message | 541.524 ms | remains signed out; 0 callbacks/private reads |

Parent browser test: 3.18 s. `model_calls=0` in every case. Private comparison
and leak-check results are booleans; no code/state/cookie/credential/reference
is exported. Each subcase gets a new recipient, cookie context and journal.
These are two loopback origins with different ports, sharing the same host/site;
they do not qualify public cross-site HTTPS or Google cookie behavior.

## Failures retained and fixed

The first browser pass failed normal/delayed return (8.812/8.742 s), while the
other three cases passed. The second pass failed the same two (8.611/8.729 s).
A focused diagnostic pass failed normal (8.577 s): provider POST returned 400,
no callback or authenticated read. These failures are not counted as successful
logins and no native Input was replayed.

Chromium exposed two fixture assumptions absent from HTTP unit clients:
`form-action 'self'` also fences the provider's redirect, so the policy now
allows only the configured RP alongside self; and `no-referrer` makes Chromium's
navigation POST Origin null, incompatible with the exact-origin form guard.
`strict-origin` preserves the POST origin while suppressing path/query in
referrers. The exact origin/redirect/CSRF checks remain required. Promise waits
now attach concurrently to avoid unhandled rejections on failed return.

## Timing interpretation

The earliest recorded operator timestamp for this slice is **07:01:34 UTC
(14:01:34 Tomsk)**. Time since that timestamp includes model deliberation,
inspection, implementation, failed probes, corrections and reporting. It is
an operator work window, not a native login duration. The final inclusive
checkpoint is recorded in the issue/PR after CI; setup before that timestamp
cannot be reconstructed reliably.

Per-case values above include process/browser startup and teardown, local
fixture requests and assertions. They contain no model inference or owner
waits. Do not use them to claim the agent can perform SSO in 0.65 s. Existing
native login measurements are preserved in the earlier scoped-login report;
they were not rerun for these metrics.

## CI and pending product work

Previous scoped-login evidence commit
`c77fa4d18332e6bcbe820aca7926e2354c0d020f`: all five workflows completed
successfully, verified during this slice:
[vault 41017047907](https://github.com/alex123321-maker/butler/actions/runs/41017047907),
[Web 41017047928](https://github.com/alex123321-maker/butler/actions/runs/41017047928),
[Android 41017047917](https://github.com/alex123321-maker/butler/actions/runs/41017047917),
[tool discovery 41017047930](https://github.com/alex123321-maker/butler/actions/runs/41017047930),
[communication 41017047939](https://github.com/alex123321-maker/butler/actions/runs/41017047939).

New workflow `Browser SSO fixture` executes
`scripts/dev/browser-pages-fixture-tests.sh`: unit/race, vet and all five
Chromium cases with race instrumentation, rejecting skipped/missing cases.
Its actual result is added to the PR/issue after execution; the profile's
existence alone is not CI acceptance.

Still required: durable per-page Broker metadata; explicit page/document
bindings; popup first-request admission; context-wide owner fencing/UI;
credential consent for the exact IdP page; real model/ingress/response delivery;
and external SSO with the owner's chosen dedicated **test Google account**.
See [the target contract](../architecture/browser-pages-proposal.md). Extra
pages remain closed in the production helper until the guarded path exists.
