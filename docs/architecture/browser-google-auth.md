# External Google sign-in: implementation boundary and next integration

## Implemented

`internal/externalauth` contains the private Google authorization-code provider
protocol, bounded private web-client configuration parser, S256 PKCE, independent
state and nonce, fixed public endpoints and strict RS256 ID-token verification.
It requests `openid email`, never offline access or Gmail/Drive permissions.
Provider identity data, authorization URLs/codes, client secrets and tokens are
not model observations, tool arguments or ActionEffect evidence. Provider errors
return one fixed public error with no body, URL or partial identity.

There are currently **no production callers**, no public callback route, no
owner launch button and no external-auth durable ledger. A passing package test,
created Google Cloud client or an already signed-in Chrome account does not
prove a Butler login, same-Input continuation or channel delivery. Existing
bounded synthetic SSO remains governed by
[its separate contract](browser-sso-protocol.md). Its native-form approval and
server-browser context cannot be reused for external Chrome OAuth.

## Required kernel integration (not implemented)

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
   explicitly implemented rather than borrowing a live-worker check.
5. Atomically consume the callback before the single token exchange. A lost
   response or process restart retains consumed authority and UNKNOWN; no
   exchange replay, grant refund or old Input recreation. After independently
   verified provider identity, match the pinned account through the vault's
   owner-keyed login index without decrypting its password. Email is an initial
   account match, not a stable identity key: persist the verified provider
   subject binding and reject later subject changes.
6. Identity verification alone is not RP session proof. Bind the RP session and
   its independently checked access evidence, record the result, and atomically
   ready only the same canonical Input. Normal lease reacquisition and action
   authorization still apply. Keep Task, Run, effect and delivery outcomes
   distinct. OTP or provider challenges remain protected owner handoff until
   a separate Broker-mediated multistep credential protocol is implemented.

The public RP must expose only exact start/callback paths through a private
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

At the current implementation boundary the project exists and the Google app
wizard is prepared; acceptance of Google's User Data Policy and OAuth client
creation are awaiting the owner's action-time confirmation. No client secret
has been created, downloaded or deployed by this change.

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

Primary provider contracts:
[Google OpenID Connect](https://developers.google.com/identity/openid-connect/openid-connect),
[web-server OAuth](https://developers.google.com/identity/protocols/oauth2/web-server),
[app audience rules](https://support.google.com/cloud/answer/15549945?hl=en).
