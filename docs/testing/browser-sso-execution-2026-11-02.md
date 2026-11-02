# SSO execution implementation and validation boundary

Issue #164, branch `codex/browser-sso-execution`, base
`065a7d55773e2bb688cdecf99b4f9f768092d882`.

This stage adds the separate protected SSO request/execution protocol,
all-of RP/IdP action scopes, one-call use ledger, canonical Input wait/resume,
private native popup plan/submit, and Go-derived RP access proof. Ordinary
page and POST policy remains separate. See the
[protocol](../architecture/browser-sso-protocol.md).

Local Windows disposable PostgreSQL/race selection completed with 37 test/subtest
passes, zero failures and zero test skips (`sso-prepublish.jsonl`, repo-local
cache). The selection includes current-scope consent rejection, protected owner
API privacy, immutable one-call contention, same-Input decision/context loss,
strict public inputs and RP/IdP authority. Empty package selections are not
counted as tests. Additional typed-proof negative cases passed separately.

The required Linux CI script is `scripts/dev/browser-sso-execution-tests.sh`.
It checks ordered production migrations and a real two-origin HTTPS recipient
through actual Chromium. Only a disposable test process and fresh NSS HOME trust
the test CA. It rejects required skips and explicitly requires positive login,
wrong password, denied IdP POST path, two-popup ambiguity and stale RP cases.
At commit `f0bf9fd5486a64be5602c7ed5c304acb05305f47`, the required
[HTTPS execution CI](https://github.com/alex123321-maker/butler/actions/runs/41404595557)
passed all 35 test/subtest cases with no skips. This includes ordered migrations,
owner API, same-Input resume and pre-claim dispatch fences. An earlier run found
a two-popup race across the private phases; the attempt guard now survives the
phase boundary and the required case confirms zero recipient POSTs.

Mechanical case wall times from that run (fixture startup + browser protocol,
`model_calls=0`, owner wait zero): verified 878 ms, wrong password 725 ms,
denied IdP POST path 558 ms, two popups 729 ms, stale RP 548 ms. Explicit IdP
HTTP rejection now ends without waiting for the 12-second popup-return timeout.
These measurements are not native agent login speed.

Remaining acceptance: actual Broker/resource/PG/runtime composition, hostile
and runtime-loss boundaries across the complete pipeline, native model attempt
with one protected consent and delivered response, then dedicated Google test
account. Do not close #164 based on these component checks. Native JS-only
forms, OTP and external Google compatibility are not established.

Timing: engineering work began 2026-11-02 05:34:35 UTC. All checks above are
mechanical (`model_calls=0`); they do not measure agent SSO speed. A native
attempt must retain ingress and delivered-response timestamps, total wall time
including model deliberation, and separately reported owner wait. Do not replay
past attempts merely to reconstruct old metrics.

Deployment check: both `laba` filesystems are full (about 12 MB free on `/`,
23 MB on `/srv/storage` at inspection). This stage has not deployed binaries or
started a native attempt on that host. The dedicated `devops` stand has space,
but a second public IdP DNS name/certificate is not yet configured.
