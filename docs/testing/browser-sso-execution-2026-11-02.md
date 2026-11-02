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
At initial publication these runtime results remain pending; compilation is
not execution evidence.

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
