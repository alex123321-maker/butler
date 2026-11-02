# Private Google identity protocol

This package implements the provider-side authorization-code exchange only.
It is not a tool, callback server, owner-consent store, credential resolver or
permission to resume an Input. It currently has no production callers.

`ReadGoogleWebConfig` reads a privately downloaded **web** client file with one
independently pinned HTTPS callback and no JavaScript origins. `NewGoogle`
accepts trusted operator configuration. Formatting and JSON serialization of
private values redact their contents. The caller must keep files, request URLs,
codes, token bodies and verified identities outside logs/model/tool outputs.

`NewChallenge` generates independent state, nonce and PKCE verifier values.
`Launch` requests only `openid email` from the fixed Google authorization
endpoint; it requests no offline access. `Exchange` makes one token POST and
one bounded JWKS GET, using the existing guarded public-DNS/TLS transport.
It accepts only a correctly signed RS256 Google ID token for the exact client,
nonce and recent attempt, with a verified email. Access and refresh tokens are
discarded. Redirects, provider diagnostics and partial identities are rejected.

The owning kernel **must** atomically consume the exact state and browser-cookie
binding before calling `Exchange`, retaining a one-use claim after any error.
This package has no durable replay protection by itself. It cannot decide
whether an owner approved an attempt, whether Task/mandate/account authority is
current, whether an RP session exists, or whether a response was delivered.
Those requirements and remaining implementation are described in
[the external Google protocol](../../docs/architecture/browser-google-auth.md).

Targeted validation (initialize repo-local Go caches first):

```
go test -race ./internal/externalauth -count=1
go vet ./internal/externalauth
```

Tests use synthetic signed tokens and an internal test transport, not a Google
account, model, PostgreSQL callback ledger or native end-to-end login. No new
dependencies are required.
