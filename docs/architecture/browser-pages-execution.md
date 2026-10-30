# Managed browser pages: implemented bounded execution

This is the implemented same-origin, credential-free stage of #164. It does
not implement federated sign-in. The separate legacy single-page tools and
one-use `browser.login` contract retain their existing semantics. A deployment
must apply migrations107/108 and update Broker, browser runtime and Web together
before advertising the new contracts. No deployment is implied by this document.

## Tools and exact authority

`browser.open_pages` creates a managed context. `browser.pages` refreshes its
page references without exporting page bodies. `browser.page_snapshot`,
`browser.page_click`, `browser.page_fill`, `browser.page_type` and
`browser.page_close` require an exact `{page_id, document_id,
observation_version}`. Contracts have revision1, strict schemas and no credential
refs. Catalog exposure requires the Broker browser-control composition. Legacy
page operations are denied on managed contexts; `browser.close` still closes
the whole context.

The Action Service resolves the exact page from PostgreSQL and hashes its
reference, safe URL, context epoch and private runtime version into the ordinary
action binding. Current Task/constraints/mandate/lease, deny rules, approvals and
grant consumption still apply. The approval summary identifies the URL and
opaque page ID. UI page selection, opener relationships, titles and URLs never
select an agent target or authorize an action.

Broker re-resolves the exact reference before dispatch. Private metadata carries
both the durable public version and the distinct executor counter. Go verifies
the private binding, Run and context epoch; Chromium verifies the same page,
document, runtime counter, URL and live DOM revision before input or close.
There is no active-page fallback. On return, Broker locks current Task/Run and
the context, validates every report against current Task policy, translates
executor counters to PostgreSQL-owned observation versions and scrubs all bytes
on rejection. Private reports/counters do not enter model results.

## Context and page lifetime

All pages retain one context identity, owner, expiry and control fence. Where
the existing resource-dispatch composition is configured, every page operation
uses the same context resource mapping. A popup cannot reset budgets, grant
authority, repair a lost executor or bypass UNKNOWN. A bare standard runtime
does not acquire a new resource kernel simply by enabling this contract.

Caps are four pending/ready pages and 32 identities over the context lifetime.
The executor generates opaque page/document IDs independently of page script.
Closed/lost identities remain terminal in the ledger. Migration108 records the
context's fixed origin/managed mode and a private runtime counter per page.
Its down migration refuses retained managed execution state.

## First-request admission

Only the initial origin is admitted, including redirects. Existing public
DNS/port/TLS/egress checks still apply. New pages must have a live exact opener
inside the current bounded gesture. Chromium can expose an initial Request
before `Request.frame()` is available. Such requests require a finite GET-only,
same-origin reservation under that gesture, bounded by the remaining live and
lifetime slots. Redirect correlation uses the actual Request, with at most five
redirects and a two-second deadline. The actual page/frame must subsequently
match that reservation. No fields or credentials can be used before readiness.
The agent gesture permits a 250ms popup admission window; late creation outside
it is rejected. Network writes bind the selected page, then freeze again.

An unobserved page has no usable reference. Failed observation closes its live
executor instead of stamping an earlier document fresh. Query/fragment values
are retained only in private masking state and removed from released URLs.
Secret collection spans all pages and frames; it does not copy cookies or DOM
between origins. Sockets and service workers retain the existing restrictions.
This stage grants no #165 WebSocket capability.

## Owner UI

The owner API returns bounded safe page metadata and frames tagged with page and
document IDs. The Web selector is a viewing cursor. Input binds that exact
page/document and the existing one-use frame token/controller/epoch. Switching
pages discards queued input and previous frame capabilities. Document changes
discard queued keys. Takeover and return fence the whole context, including
pending popups. After return the agent must refresh `browser.pages`; old grants
and observations cannot resume input. Owner input remains private.

## Effects, credentials and remaining gates

All page mutations require ordinary approval and use the existing unsafe/none
effect policy. A helper success is not recipient confirmation. A completed
opaque mutation can remain UNKNOWN and block every page's next write. This
stage does not extend legacy frozen-snapshot owner factual review to managed
pages; it does not invent absence, replay input, clear history or refund budget.
Page-specific effect verification/review is a remaining contract before a
general autonomous multi-step managed-page workflow can be qualified.

Credential requests against managed contexts are denied; password fields cannot
accept ordinary literals. RP/IdP authorization, protected page-aware consent,
cross-origin transitions, authenticated RP evidence, anonymous negative control,
native model execution/delivery and the owner's dedicated Google account are
remaining acceptance. Do not reuse provider control-plane tokens for Google.

## Verification

`scripts/dev/browser-page-execution-tests.sh` requires isolated PostgreSQL and
both pinned Chromium installations. It fails required skips and exercises
ordered migrations, page authority and retention, real Chromium targeting,
first requests/caps/runtime loss, and actual Web -> production owner API ->
Broker -> resource kernel -> PostgreSQL -> runtime. The Windows owner UI
equivalent is `scripts/dev/browser-page-execution-local.ps1`.
Mechanical tests have zero model calls. Native latency includes model
deliberation and owner waits and must be measured separately; see the
[execution evidence](../testing/browser-page-execution-2026-10-30.md).
