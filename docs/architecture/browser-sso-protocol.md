# Protected RP/IdP consent

## Consent and execution contract

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

With a configured vault, Broker registers `credentials.request_sso_login` and
the distinct unsafe commit tool `browser.sso_login` revision1. A request suspends
and resumes the same canonical Input through the existing protected owner
decision path, without model polling. One protected SSO approval includes one
native IdP form POST and RP access verification; no generic POST approval is
added. Ordinary login and generic page tools cannot consume `ssoq_` references.
Consent alone does not waive current Task, tool, effect, denial, resource lease
or owner-goal mandate checks. RP, IdP, POST target and verification URL are an
all-of resource requirement, including path prefixes; legacy origin mandates
and delegated execution cannot authorize this initial SSO protocol.

## Bounded executor

The Broker protocol checks RP **and** IdP resource authority before
opening the fresh child of the consented exact RP document. The owner consents
to that one conditional future child, not any currently active IdP tab. The
trusted executor must privately bind its actual page/document, opener, URL and
form before inserting fields. Page capacity, egress/TLS guards, ownership/epoch,
current authority and exact consent must be checked again before secret release
and dispatch. Runtime must not fetch secrets or choose authority itself.

The bounded executor admits only the expected GET-only transition and one
compiled native IdP form POST, then bounded RP/IdP GET redirects. All unrelated
network writes stay denied. It must freeze and close the ephemeral IdP child,
retaining its identity; ordinary managed-page tools keep their existing
same-origin contract. Cookies stay in their native browser context and origin.

Broker privately transmits the exact RP binding, scope digest, consent expiry
and current allowed/denied path caps. Runtime retains public DNS, TLS, operator
domain caps, ownership, epoch, context TTL and page capacity guards. The helper
opens only one fresh child of the pinned RP document, privately binds its actual
opener/document and authorization URL, and compiles the native form before any
write. A second popup, stale source, unrelated request or unsupported form fails
closed. The plan nonce, credentials, OAuth query, CSRF, cookies and IdP DOM never
enter public results. The finite network window freezes after each private phase.

Migration110 adds an immutable `credential_sso_uses` claim binding one consent to
one Run/tool-call/canonical arguments hash. Both field resolutions recheck live
Run/Task/account/page/epoch and expiry, including after claim contention. Another
call cannot spend the consent. A claim means consumed authority, never proof of
login. Separate durable notice state prevents ambiguous channel sends replaying;
the live-state view expires stale owner prompts without rewriting the decision.

Success requires Go-derived access differential on the exact protected RP URL:
authenticated GET success and a separate anonymous negative control. Popup close,
DOM text, postMessage or IdP HTTP 200 cannot prove RP login. Ambiguous/lost ACK
must retain UNKNOWN and consumed authority; no replay or automatic refund.
The ordinary `browser_login_v1` proof does not cover federation.

Only Go's independent GET differential can produce `browser_sso_login_v1`.
Broker binds effect evidence to the exact post-login RP page/document/public
observation, digest and current epoch in PostgreSQL before confirmation. The
private runtime counters and final page reports pass the normal managed ledger.

## Validation boundary

`scripts/dev/browser-sso-execution-tests.sh` requires ordered PostgreSQL
migrations, consent/one-call/current-authority tests, same-Input wake/resume, and
real Chromium against the actual two-origin HTTPS recipient. Its private test
CA is trusted only by that process and a fresh Linux Chromium HOME via NSS;
production certificate validation and workstation trust are unchanged. Required
cases cannot skip. Mechanical timings explicitly have `model_calls=0`.

Required subsequent acceptance: composed Broker/resource/PG/Chromium execution,
hostile/late/wrong-opener popup and origin transitions, one consent/one POST,
canary privacy, current-authority and replay fences, negative access control,
native model execution and response delivery, then the owner's dedicated Google
test account. JS-only forms, OTP and Google success must not be inferred from
the synthetic fixture or from these component checks. Native wall time starts
at ingress and ends at delivered response, includes model deliberation, and
retains owner waits as a separate reported component. Engineering/CI time is
never SSO login latency; old attempts are not replayed for timing reconstruction.
