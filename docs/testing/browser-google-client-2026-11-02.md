# Google test client setup — 2026-11-02

## Cloud setup

The dedicated project `butler-browser-sso-test-261102` contains one confidential
Web application client named **Butler Browser SSO Test Web**. Creation was
confirmed in Google Cloud on November 2 at 18:32:59 Asia/Tomsk. The owner
explicitly approved the User Data Policy and this exact client before creation.
The existing unrelated project was not modified. No billing/trial was activated.

Configuration:

- App: Butler Browser SSO Test; External audience, **Testing** status.
- One dedicated test user, corresponding to the protected vault record
  `browser-google-sso-test`. No password was sent to the synthetic recipient.
- Exactly one redirect:
  `https://butler-sso-rp.report-template.ru/auth/google/callback`.
- No authorized JavaScript origins; server-side authorization-code flow.
- Configured scopes: `openid` and `https://www.googleapis.com/auth/userinfo.email`.
  No profile, Gmail, Drive, sensitive or restricted scopes were selected.
- The client represents the RP's identity sign-in, not a Google MCP/API agent.

The JSON-download wait timed out. The displayed client ID/secret were then
transferred directly inside browser automation to an operator-only local file;
they were not put in source, CLI arguments, reports or Butler model prompts.
The local file inherits access only for the Windows user and SYSTEM, and git
ignores it. The browser clipboard was cleared. The private file is compatible
with Google's web-client JSON format, but is not a verified completed download.
`ReadGoogleWebConfig` accepted it (`private_client_valid=true`).

No client configuration was deployed and no real Google authorization attempt
was started. Public callback, durable attempt/claim, protected owner launch,
current authority, RP session/access proof and canonical Input continuation
remain [implementation work](../architecture/browser-google-auth.md).

## Provider checks

Commit `32692cea7053d4c6d78b8f8f33dd8b6a84bb8eb2` adds private provider protocol
operations without new dependencies or a model-facing tool. Local Go race
checks: **36 tests/subtests passed**, zero failures or skips; vet passed.
Tests use signed synthetic tokens and an internal fake transport, not Google.

Required [SSO CI run 41437340241](https://github.com/alex123321-maker/butler/actions/runs/41437340241)
passed at that exact commit: **115 tests/subtests passed**, zero failures/skips,
including 36 new provider cases and the existing 79 consent/authority/real
HTTPS-Chromium cases. Artifact `browser-sso-execution-evidence`, ID
`13137126991`, digest
`sha256:89950ee5cd87996b5fce1879db40c2d99c1313faf1bd9867921831c293207312`.

These are mechanical tests with `model_calls=0`. Their durations do not measure
Google login latency or Butler's model deliberation. Native ingress-to-delivery
timing starts only when the complete external flow is available; prior runs
were not replayed to reconstruct metrics.
