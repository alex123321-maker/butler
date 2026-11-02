# Protected RP/IdP consent

External Google authorization uses a separate
[provider protocol and integration plan](browser-google-auth.md). Its private
provider library, Core callback, owner/authority binding and same-Input
continuation are implemented. Native-form SSO consent below
does not authorize external Chrome OAuth or generic user-tab credential access.

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

The owner UI initializes the pinned account for requests arriving during an
already unlocked panel as well as at initial opening. The account selector
remains disabled; a complete scope and explicit owner click remain required.
Polling initializes card state only and never issues approval.

## Bounded executor

Before consent, `browser.sso_inspect` revision1 permits a separate bounded
GET/HEAD-only inspection. The model selects an exact observed RP page and
button, the intended distinct IdP origin and a public protected RP link. The
Broker requires current owner-goal read authority for all three targets, no
delegation or credential references, and the current page/document/epoch.
Private dispatch pins these values, path allow/deny caps and a short expiry.
The runtime opens at most one fresh popup, admits the expected provider GETs,
and denies provider-to-RP return even when it is an immediate redirect before
the popup page event. Inspection cannot fill or POST. It closes the popup and
freezes the context before returning.

Both SSO phases fetch one authorized response at a time through the browser context's
TLS-verified proxy and private cookie jar with automatic redirects/retries
disabled. An allowed redirect becomes a minimal private refresh document,
forcing a new browser navigation through the guard before contacting its next
destination. Provider response bodies are never rendered under the RP origin.
Each private HTTP request has a fresh, initially empty, one-response cookie
store and copies Chromium's actual headers, including absence of Cookie.
The transport cannot add origin-only cookies withheld by browser SameSite or
partition policy. Response Set-Cookie headers, including on the redirect bridge,
are processed by Chromium itself rather than copied through addCookies. The
transport's private response values are included in secret redaction and its
context is disposed after that response. No API jar is reused or imported.
This is necessary because normal Playwright routes do not intercept every
redirect. A provider-to-RP redirect is denied before any callback request.
After the sole authorized POST, only its bounded returned GET redirect chain
is permitted; an additional POST/307/308 replay is denied. Actual returned
paths still require the private current allow/deny caps before contact.
The helper inherits the operator's optional `NODE_EXTRA_CA_CERTS`; the HTTPS
test uses only its isolated CA, without disabling certificate verification.

Only canonical query-free IdP/form paths and bounded positional form/input
selectors are exported as candidate `sso_scopes`, together with a fresh
PostgreSQL-owned RP observation. No IdP body, field names/values, hidden CSRF,
cookies or OAuth query is returned. Secret-reflecting paths, unsupported forms,
multiple popups, stale pages and helper loss fail closed. Broker rechecks
current authority and each discovered path before release. This is discovery,
not consent or authentication evidence; the model must choose a returned scope
and obtain the existing single protected SSO decision before submission.
Ordinary page observations expose bounded button labels/positional selectors
already visible in the page, under the same secret redaction rules.

This protocol covers native forms with publicly discoverable protected RP
navigation. It does not discover arbitrary private verification endpoints or
automate JavaScript-only providers, MFA or Google account entry.

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

An UNKNOWN SSO attempt also blocks later Inputs in the same conversation; a
new Task is not reconciliation. Historical owner review is permitted only for
the exact revision1 SSO policy/arguments, trusted RP page binding and retained
owner/Run/Task managed-context mapping. The recorded `browser.open_pages`
context must have expired, including the 35-second dispatch settling bound;
the source Run must be terminal and the conversation lease inactive. Missing
dispatch completion, altered scope, foreign owner/mapping, live context or
legacy context birth are rejected. Live generic page review cannot be borrowed.
The existing protected factual owner API requires independent recipient and
settled-processing checks plus a receipt reference. A counted test operator may
use a sealed synthetic-recipient journal through this path; that is explicitly
owner-assisted evidence, never an automatic production verifier. Review does
not resume the failed Input/Run, replay/refund its consumed grant or rewrite
the original UNKNOWN tool transcript. Any new login needs new current authority
and a new protected credential decision.

Only Go's independent GET differential can produce `browser_sso_login_v1`.
Broker binds effect evidence to the exact post-login RP page/document/public
observation, digest and current epoch in PostgreSQL before confirmation. The
private runtime counters and final page reports pass the normal managed ledger.

## Validation boundary

Required runtime fault cases kill the actual helper before SSO or after the
independent synthetic recipient has synced an accepted IdP login while delaying
its callback. Neither case can emit authentication proof. The old context must
reject another SSO invocation; a fresh read must have a different identity and
no restored authentication. The accepted-login journal cannot change from that
stale invocation or fresh read. The recipient's bounded operator-only delay
does not exist in the production runtime. Delayed-popup success and popup-storm
denial are separate controls. All these checks are mechanical (`model_calls=0`),
not a native Core/Broker recovery qualification or permission to reconcile an
unknown action automatically.

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
