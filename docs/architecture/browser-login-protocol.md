# Bounded browser login protocol

This is an implementation contract for `browser.login`, not new semantics for
opaque click or generic POST. A new `login` vault request explicitly asks the
owner to fill and submit one named login form and verify authenticated access.
Legacy access/create/OTP grants retain their original preparation-only rights.

The request pins current owner/session/Input/run/Task/constraint versions,
record revision, browser session, expected page URL, exact same-origin POST
target, form selector and username/password selectors. Owner UI renders this
scope before allowing it. The submitted args contain two deferred references to
the same approved request, never values. Broker policy, autonomy, task effect
and tool permissions, expiry, record revision and final dispatch fences still
apply. The approved request may authorize this exact login protocol without a
second action approval; it cannot authorize click, another form, payment, storage
import or another site's credentials. A durable call claim prevents another
proposal from spending the same login permission. No ambiguous dispatch retries.
The protected response must explicitly carry `login_authorization=browser_login_once_v1`.
Old open clients that render unfamiliar kinds as access-only cannot approve a
login request. Denial remains available without this marker; PIN and owner auth
remain mandatory. The marker is an explicit consent version, never a credential.

The reviewed browser helper freezes network before inspecting the form. It
supports a top-level ordinary same-origin URL-encoded POST form with one bound
username and password input and bounded, allowlisted hidden CSRF fields. Other
hidden business/action fields are unsupported and cannot borrow login consent.
Selectors support unique native CSS and the bounded `CSS >> nth=N` form emitted
by browser observations. Unindexed ambiguous matches fail closed. The same
selection is rechecked in planning and submission; other selector engines are
unsupported and cannot widen the frozen consent scope.
It fills privately and
uses native form submission, not click/submit handlers. Routing permits at most
one matching top-level POST with the exact compiled fields and bounded same-origin
GET redirects; other requests stay blocked. Unsupported or stale forms fail
closed. DOM messages, HTTP 200 alone and helper assertions are not login proof.

After submission the Go runner independently probes the observed new landing
URL through the existing public-DNS/redirect-safe HTTP transport. The browser's
private cookies are used only for an authenticated GET; a separate empty-cookie
GET must receive 401/403 or redirect back to the pinned login page. Authenticated
access must return 200 without a login redirect, and all targets stay in origin.
No credential/body/header/cookie values enter the model or effect journal.
This proves an observed access differential for this exact authentication
procedure, not the site's business semantics or a universal remote-write receipt.
Browser observation remains the source of user-facing page data.

Only the Go-derived, correlated `browser_login_v1` result can confirm the typed
authentication effect. Its precise policy, request scope, browser identity,
version and proof are rechecked by Broker/PostgreSQL. Normal verified login can
continue and deliver its answer without an owner factual report. Missing proof,
public landing pages, cross-origin navigation, lost response/runtime or changed
authority retain UNKNOWN and the existing no-replay protection. OTP, JavaScript-
only forms and popup SSO are outside this first protocol and do not borrow its
automatic confirmation. General clicks and POSTs retain their approval policy.

A terminal lost-response login can use the existing protected factual report
only after its original browser context and runtime call window have settled,
with all historical identity/policy guards in the external-effect journal
contract. This never resumes that Input or reissues its consumed login grant.

Acceptance requires real Broker/PG/Chromium tests for one consent/one POST,
private canaries, authenticated versus anonymous access, existing-grant denial,
replay/concurrency, stale/foreign/expired/revoked scope and unsupported forms.
A native ordinary-ingress/provider/delivery run must use no test-oracle report.
