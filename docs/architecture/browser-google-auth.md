# External Google sign-in: bounded diagnostic RP contract

## Implemented

`internal/externalauth` contains the private Google authorization-code provider
protocol, bounded private web-client configuration parser, S256 PKCE, independent
state and nonce, fixed public endpoints and strict RS256 ID-token verification.
It requests `openid email`, never offline access or Gmail/Drive permissions.
Provider identity data, authorization URLs/codes, client secrets and tokens are
not model observations, tool arguments or ActionEffect evidence. Provider errors
return one fixed public error with no body, URL or partial identity.

Core, Broker and owner UI implement a separate, **default-disabled** external
handoff for an operator-configured diagnostic RP. Migration 111 retains its
immutable request, encrypted challenge, one-use launch, exchange claim, result,
stable identity binding and independent notification state. A passing package
test, created Google Cloud client or an already signed-in Chrome account does
not prove deployment, native login or channel delivery. Existing bounded
synthetic SSO remains governed by
[its separate contract](browser-sso-protocol.md). Its native-form approval and
server-browser context cannot be reused for external Chrome OAuth.

## Kernel and owner decision

1. Broker validates a distinct external-auth proposal against current owner,
   Session, Run, Task/constraint versions, selected record revision and exact
   RP/IdP resource authority. Creating or saving an account grants no use.
   No generic `single_tab.*` credential release or native-form scope expansion.
2. PostgreSQL retains an immutable attempt identity and protected owner decision,
   with expiry and encrypted state/nonce/PKCE material. Owner UI shows provider,
   RP, pinned account and identity scopes. One explicit decision covers this
   one code exchange and verification, without a second generic POST approval.
3. A private owner-only launch response produces a one-use handoff ticket.
   The exact public RP start route consumes that ticket, sets a per-attempt
   Secure/HttpOnly/SameSite=Lax browser cookie and redirects to Google in normal
   Chrome. No state/code/token enters a transcript, public report or telemetry.
4. The exact callback validates method, bounds, unique state/code (or provider
   denial), cookie, expiry, pinned client and original attempt. Recheck current
   owner/Task/mandate/account/canonical Input authority transactionally. Waiting
   Inputs do not have an active worker lease; callback authorization must be
   checked through its canonical suspended Input rather than borrowing an
   active worker lease. Callback replay is serialized before Task/Input locks
   so a losing callback cannot deadlock the winning budget reservation.
5. Atomically consume the callback before the single token exchange. A lost
   response or process restart retains consumed authority and UNKNOWN; no
   exchange replay, grant refund or old Input recreation. After independently
   verified provider identity, match the pinned account through the vault's
   owner-keyed login index without decrypting its password. Email is an initial
   account match, not a stable identity key: persist the verified provider
   subject binding and reject later subject changes, including case changes.
   An initial email match additionally requires Google-authoritative ownership:
   Gmail, or a verified email with a signed hosted-domain claim. A third-party
   address with `email_verified` alone cannot establish this binding. See
   [Google's identity-verification contract](https://developers.google.com/identity/gsi/web/guides/verify-google-id-token).
6. Identity verification alone is not RP session proof. Bind the RP session and
   its independently checked access evidence, record the result, and atomically
   ready only the same canonical Input. Normal lease reacquisition and action
   authorization still apply. Keep Task, Run, effect and delivery outcomes
   distinct. OTP or provider challenges remain protected owner handoff until
   a separate Broker-mediated multistep credential protocol is implemented.

`credentials.request_external_login` is prepare-only. It requires an explicit
current owner goal/mandate covering both prepare and commit for the exact RP,
verification path and Google's fixed authorization/token/JWKS destinations.
One PIN-protected owner answer binds the immutable scope digest and selected
record revision. It grants no generic HTTP POST, native form grant, credential
alias, password resolution or arbitrary provider URL. Broker has only the
non-secret profile; only Core receives the private client file.

The trusted Broker dispatch identity names the **permanent effect**, which may
differ from the current execution intent under a logical Task slot. Request
creation derives the execution ID from Run/call and validates its intent against
that separate effect ID, fingerprint and exact dispatch record. Neither ID can
stand in for the other. The composed Broker/PG test must use a logical slot so
this distinction is exercised rather than collapsed by a fixture.

Before the single provider exchange, Core reserves a separate finite execution
budget and commits an ActionEffect child with `external_google_rp_v1` policy.
The child cannot be dispatched through the prepare-only public tool contract.
Google identity is matched privately, then the exact RP account endpoint is
read with the per-attempt cookie (200) and without it (401). Only this
differential can produce `verified` and the independently generated RP
reference. Provider identity/token bodies are never persisted. Task and account
authority, source document/version/epoch and canonical Input are checked again
after network waiting; Task locks are not held across provider requests.

Approval alone does not ready the Input. A terminal verified/unverified result,
denial, expiry, stale source or cancellation wakes only that same Input. An
already-issued private link can be recovered after an owner-page reload only
while its original ticket is unspent. No second approval or new ticket is
created. The owner can cancel before or during exchange. A pre-dispatch budget
rejection closes the attempt without contacting Google. Cancellation after
dispatch, lost token/probe acknowledgement or abandoned exchange becomes
retained UNKNOWN, never an inferred failure or refundable grant. A settled
exchange is swept after the bounded execution window, including across restart.
Unresolved private-auth effects block later writes in the conversation, even
under a new Input/Task; independent reads and the existing exact owner-report
publication contract remain available. Historical owner factual review uses
the existing protected outcome UI only after the worker, Input, Run and source
context are settled. It may change the journal assessment, but cannot rewrite
the original callback result, resume old work, replay state or refund spending.

The owner UI opens normal Chrome using a private one-use URL returned only by
the protected owner API. Popup blocking has an explicit private-link fallback;
the opener is removed and navigation is restricted to the pinned HTTPS RP
start route. The RP sets no-store/no-referrer/CSP headers and exposes only
root/start/callback/account. Callback query values and errors are not reflected.
Owner notification claims are durable and an ambiguous send is not resent.

This RP is deliberately a diagnostic application on an owned host. A normal
Chrome session is **not copied into server Chromium**, and this flow does not
provide general authenticated DOM continuation on arbitrary third-party sites.
An existing Google Chrome session proves neither vault password injection nor
MFA automation. These limits must be present in external acceptance evidence.

The public RP must expose only exact root/start/callback/account paths through a private
relay. Do not expose owner APIs, relax TLS/network guards, or send a Google
password to the synthetic IdP. Private downloaded client configuration belongs
in a restricted operator secret mount, never git, fixture configuration or logs.

## Isolated client configuration

The dedicated project is `butler-browser-sso-test-261102` (Butler Browser SSO
Test). The intended client is a confidential **Web application**, with exactly
one authorized redirect:

`https://butler-sso-rp.report-template.ru/auth/google/callback`

No JavaScript origin is needed for this server-side code flow. The app stays in
Testing; the backend must independently enforce the selected dedicated account.
Google exempts identity-only scopes from parts of its test-user restrictions,
so a test-user list is not a Butler access-control boundary. Do not activate
billing or publish the app as part of this test.

The owner approved Google's User Data Policy and creation on 2026-11-02.
`Butler Browser SSO Test Web` is created, the app remains in Testing, one
dedicated test user is listed, and only `openid`/`userinfo.email` are configured.
The client secret is retained in a restricted, git-ignored local operator file;
the provider configuration parser accepted that file. Setup evidence is
[recorded separately](../testing/browser-google-client-2026-11-02.md); client
creation alone is not deployment or external smoke acceptance.

## Acceptance

Required before enabling: durable concurrency/replay/expiry/revocation tests;
wrong browser cookie, account, subject, state, code, client and current-version
negative controls; restart after claim/lost token ACK with no replay; actual
owner-UI and Broker integration; exact public-route isolation; safe canary
checks; real Google Chrome login with independent RP access proof; same-Input
resume and actual Web/Telegram delivery.

The native timing window begins at ingress and ends at delivered response,
including model deliberation and failed actions. Owner/Google challenge wait is
reported as a component of that inclusive total. Engineering/CI time is separate;
do not replay older attempts to reconstruct timings.

If the pinned request-creation operation loses its acknowledgement before any
request exists, the terminal kernel can prove metadata creation ABSENT using
the [stopped review](stopped-input-finalization.md). This checks both permanent
effect and execution IDs, exact historical contract and original terminal
fences. It never infers absence of a provider exchange, reuses consent, replays
the failed Input or refunds spending. A fresh attempt needs fresh authority.

Primary provider contracts:
[Google OpenID Connect](https://developers.google.com/identity/openid-connect/openid-connect),
[web-server OAuth](https://developers.google.com/identity/protocols/oauth2/web-server),
[app audience rules](https://support.google.com/cloud/answer/15549945?hl=en).
