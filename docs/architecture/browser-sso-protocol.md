# Protected RP/IdP consent

## Implemented foundation

`browsersso.Scope` describes one conditional SSO attempt: an exact ready RP
page/document/observation, RP trigger, distinct IdP login URL and native form
target/selectors, and exact protected RP verification URL. URLs are canonical
HTTPS without userinfo, query or fragment. OAuth state, authorization codes,
PKCE material and private query strings are not part of this public scope.

Migration109 retains a separate `credential_sso_requests` ledger. It pins the
owning owner/Session/Run/Task and task/constraint revisions, selected account
revision, browser context epoch and complete scope. Creation checks the current
managed RP page in PostgreSQL and clips the ten-minute request TTL to context
expiry. Exact same-call repeats retain identity and expiry; changed proposals
cannot replace the scope. Rows cannot be deleted, truncated, repurposed or moved
back to pending. A downgrade refuses retained records. Account deletion remains
possible; the ledger retains only its opaque account ID, not encrypted fields.

The existing owner-authenticated, PIN-unlocked `/api/v4/credentials` endpoint
lists safe SSO metadata and accepts one protected decision. The card displays
RP, IdP, account, POST target, access check, popup cleanup and exact RP document.
Approval requires `login_authorization=browser_sso_login_once_v1` plus the
displayed `scope_digest`. That digest pins the exact request identity, owning
principal/versions, purpose, account, scope and context generation; it is a
consistency marker, not owner authentication or a secret. Equal scopes on two
different requests have different card digests.
An old generic-access client, ordinary-login marker, changed account or stale
card cannot approve. Approval rechecks current Run/Task, live RP page and epoch,
expiry and active account revision transactionally. Repeating the same protected
answer records no second decision or wake. Denial requires no approval marker.
Request expiry is checked again against database time after acquiring the
decision row lock; waiting for another transaction cannot extend consent TTL.

This is consent storage and protected presentation, **not an installed SSO
executor**. No model-facing request/execute tool is registered at this stage.
`ssoq_` IDs are not legacy credential aliases; `GetByAlias`, `CheckGrantedCall`
and `ResolveField` cannot release their fields to any browser tool. Existing
managed-page credential denial and generic POST approvals remain unchanged.
Consent alone authorizes neither RP nor IdP network access and cannot waive
current Task/mandate resource, tool, effect or denial checks.

## Next execution gate

The future Broker protocol must check RP **and** IdP resource authority before
opening the fresh child of the consented exact RP document. The owner consents
to that one conditional future child, not any currently active IdP tab. The
trusted executor must privately bind its actual page/document, opener, URL and
form before inserting fields. Page capacity, egress/TLS guards, ownership/epoch,
current authority and exact consent must be checked again before secret release
and dispatch. Runtime must not fetch secrets or choose authority itself.

The bounded executor must admit only the expected GET-only transition and one
compiled native IdP form POST, then bounded RP/IdP GET redirects. All unrelated
network writes stay denied. It must freeze and close the ephemeral IdP child,
retaining its identity; ordinary managed-page tools keep their existing
same-origin contract. Cookies stay in their native browser context and origin.

Success requires Go-derived access differential on the exact protected RP URL:
authenticated GET success and a separate anonymous negative control. Popup close,
DOM text, postMessage or IdP HTTP 200 cannot prove RP login. Ambiguous/lost ACK
must retain UNKNOWN and consumed authority; no replay or automatic refund.
The ordinary `browser_login_v1` proof does not cover federation.

Required subsequent acceptance: actual Broker/PG/Chromium two-origin execution,
hostile/late/wrong-opener popup and origin transitions, one consent/one POST,
canary privacy, current-authority and replay fences, negative access control,
native model execution and response delivery, then the owner's dedicated Google
test account. JS-only forms, OTP and Google success must not be inferred from
the existing synthetic fixture or from this consent foundation.
