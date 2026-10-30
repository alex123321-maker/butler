# Browser pages and federated sign-in: proposed next contract

Status: **target design for #164, not implemented production behavior**. The
current helper still closes extra pages and Broker stores one observed URL per
context. This change adds only the isolated
[SSO recipient and Chromium probe](../../cmd/browser-sso-eval/README.md).
Existing browser-session, credential and scoped-login contracts remain active.

## Identity and authority

The target retains one `browser_session_id` owned by the trusted Run/Task and
adds opaque `page_id` values for a bounded set of pages (initial cap: four).
Page identity never derives from title, URL, array index or an active-window
cursor. Every new page-aware read, preparation, gesture and credential request
must name the page and bind its current document/version. Opener relationships
and page lifecycle events are observations, not authorizations.

Broker needs durable per-page metadata before the helper can admit popups:
session/page identity, safe observed origin/path, current document version,
context/control epoch, owner/Run/Task and lifecycle state. A tool must resolve
authority against its exact page, recheck returned metadata and scrub denied
observations. Never use the current single session URL to authorize another
page. Raw query/fragment values, userinfo, callback code/state and cookies must
not enter model observations, PostgreSQL metadata, logs or owner event feeds.
Any required exact callback comparison stays in the private executor.

New tools need full schema/discovery/prompt/Broker/runtime/owner-UI wiring.
Do not partially extend the private helper and advertise support. Legacy
single-page semantics remain unchanged until the complete guarded path exists.
No public tool names or schema are registered by this proposal.

## One context remains one control lane

All pages share the context resource lane, expiry and takeover fence. Opening a
page cannot escape an UNKNOWN external effect, reset budgets, create another
owner or obtain a new write grant. Close and runtime loss invalidate the
affected page/document; the existing lost-context and no-replay rules apply.

Owner UI page selection is a viewing cursor, not execution authority. Takeover,
stop/revoke and return fence every page and pending popup. Frame/control tokens
bind page, document and context epoch. Return requires fresh observations before
any further input; previous selectors, preparations and credentials are stale.

## Popup admission and network policy

Popup creation may begin its first request before a Playwright page event.
Admission therefore must fence requests from unregistered pages at the network
boundary, then bind the admitted page under current Broker authority. Do not
admit an initial request by relying only on a later `context.on('page')` handler.
Exercise initial about:blank, early/late navigation, popup close races and storm
limits in actual Chromium. Overflow pages must close without making an
unauthorized request or becoming an untracked credential target.

Each connection and redirect keeps public DNS/port/TLS/egress enforcement.
Federation does not grant arbitrary origins to an opener, redirect or popup.
Do not add a context-wide allow-write flag. Service workers, WS/SSE and existing
connections remain governed by their own #165 admission/lifetime contract;
stopping a context must prevent background work across all its pages.

## Credentials and completion

Credential mediation binds the exact account, page/document, approved IdP
origin and current authority version. A trusted RP opener does not authorize
credentials on its popup. Existing one-use same-origin `browser.login` consent
does not silently become cross-origin SSO consent. The future federation
contract must state the RP, IdP and allowed one-use sign-in transitions without
weakening unrelated POST protection.

Secret masking must collect private fields and session/callback values across
all pages and frames, including closed popup transitions. SOP and native cookie
partitioning remain Chromium responsibilities; no DOM or cookie copying may
simulate successful login. Completion requires authenticated RP evidence and
an anonymous challenge control, independently receipted by the RP. A popup
closing, message arriving, provider cookie or successful IdP POST is insufficient.

## Acceptance sequence still outstanding

1. Implement page/document contracts, Broker storage/policy and context fencing;
   include negative policy/storage tests before enabling popups.
2. Wire helper, discovery and owner UI. Verify initial request admission, equal
   titles, early/late/closed pages, stale preparations, owner switching,
   storm caps, redirect/private-address and foreign-Run denial in Chromium.
3. Deploy an isolated HTTPS RP/IdP pair through ordinary production egress.
   Run the real model through ordinary ingress, protected vault consent,
   authenticated RP read and actual response delivery. Retain failed/UNKNOWN
   history and count wall time including deliberation and owner waits.
4. External smoke with the **dedicated test Google account** selected by the
   owner. Use a suitable Google-enabled RP/client and protected owner interaction;
   do not reuse control-plane provider tokens or send codes/passwords in chat.
   This step is not executed by the synthetic fixture or this proposal.

Issue #164 stays open until its production/native/external criteria have actual
evidence. #165 networking and #168 OTP/auth persistence are separate work.
