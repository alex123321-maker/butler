# Two-origin SSO evaluation recipient

Operator-only Go/standard-library fixture for issue #164. This command is not a
Butler tool, an OAuth/OIDC implementation, or an authentication provider for real
accounts. It does not enable production popups, grant authority, invoke a model,
or exercise Google sign-in.

One process serves a relying party (RP) and a synthetic identity provider (IdP)
on separate origins. The RP's `/account` is unavailable until a browser-bound,
unexpired, one-use code/state callback succeeds. Provider login alone, a forged
message, or closing a window does not authenticate the RP. The simplified
code/state flow binds a private verifier with an S256 challenge; it has no token
endpoint, OIDC identity claims, consent scopes or discovery protocol.

## Private configuration and serving

Initialize repo-local caches using `scripts/dev/common.ps1` on Windows. Generate
synthetic data and start two loopback listeners:

```powershell
go run ./cmd/browser-sso-eval -init -config .cache/sso-private.json
go run ./cmd/browser-sso-eval -config .cache/sso-private.json -journal .cache/sso-attempt-01.jsonl -rp-origin http://127.0.0.1:18098 -idp-origin http://127.0.0.1:18099
```

Defaults: `-rp-listen 127.0.0.1:18098`, `-idp-listen 127.0.0.1:18099`.
Configuration and journal are exclusively created; existing files are never
overwritten. Restart with a new journal, retaining the old attempt. All live
bindings expire after three minutes and are not restored after restart.

Keep these files out of model context, chat, logs, source control and artifacts.
Do not use real credentials. Go's file mode 0600 does not establish Windows
ACLs: place private files in an appropriately restricted directory. Generated
credentials for a future native test must enter through the protected owner
vault, not tool arguments.

The public topology requires two distinct HTTPS origins at a trusted TLS
terminator forwarding each to its own loopback listener. The configured origin
controls Host validation and Secure cookies; forwarded headers cannot change
it. There is no public deployment in this change. Isolate ingress before any
deployment: this short-lived synthetic recipient is not a public identity
service. Production egress remains unchanged; loopback test destinations do not
become available to Butler.

State is bounded: 128 RP/IdP browser bindings each, 64 pending flows/forms each,
128 codes/accounts each. CSRF tokens and code/state are one-use, browser-bound
and expiring. POST requires the exact IdP Origin, a bounded form body and exact
fields. The callback fixes its redirect target and rejects duplicate parameters,
foreign browsers, proof mismatch and replay. Cookies are HttpOnly, SameSite=Lax
and Secure for HTTPS. Callback pages contain no code/state values in their body.

The recipient journal contains only sequence, UTC timestamp, fixed event kind
and acceptance boolean. No URLs, code/state, cookies, credentials or private
reference are recorded. When supported, a receipt is synced before releasing a
code, authenticated cookie or private reference. Journal failure latches the
recipient unavailable; it cannot grant access without a receipt.

## Verification and limits

`bash scripts/dev/browser-pages-fixture-tests.sh` requires Go, Node and the
existing pinned tool-browser Playwright/Chromium installation. It runs race
tests, vet and all five actual Chromium cases, rejecting any skipped case.
Reports are safe JSONL under `.cache/browser-pages-ci/`.

The probe receives generated synthetic fields through a private stdin pipe and
exports only fixed booleans, HTTP status codes and durations. It blocks other
origins, WebSockets, service workers and downloads. It is a mechanical fixture
probe, not a model-controlled or Broker-mediated runtime. Its loopback servers
have different ports (different browser origins), but share the same site/host.
Cross-site cookies and public HTTPS are not qualified by this profile.

Cases: normal popup login; delayed popup; two distinct pages with identical
titles; close before login; forged completion message. Positive cases require
one accepted provider login, one RP callback and one private account read in the
independent Go recipient, plus an anonymous 401 control. Negative cases must
produce no accepted RP callback or private read. The popup-storm button is a
future runtime-cap fixture, not a passing bounded-pages test in this change.

Component durations include the entire test/probe execution and have zero model
calls. Native Input timing must separately include model deliberation, owner
waits and final delivery. Never replay previous attempts to backfill metrics.
See the [page-addressing proposal](../../docs/architecture/browser-pages-proposal.md)
for production work still required before native and external Google acceptance.
