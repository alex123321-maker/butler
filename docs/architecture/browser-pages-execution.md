# Managed browser pages: implemented bounded execution

This is the implemented same-origin, credential-free stage of #164. It does
not implement federated sign-in. The separate legacy single-page tools and
one-use `browser.login` contract retain their existing semantics. A deployment
must apply migrations107/108 and update Broker, browser runtime and Web together
before advertising the new contracts. No deployment is implied by this document.

## Tools and exact authority

`browser.open_pages` creates a managed context. `browser.pages` refreshes its
page references without exporting page bodies. `browser.page_snapshot`,
`browser.page_freeze_snapshot`, `browser.page_click`, `browser.page_fill`, `browser.page_type` and
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
stage now supports the bounded live factual review below; it does not invent
absence, replay input, clear history or refund budget. Autonomous recipient
verification remains separate from an authenticated owner's factual report.

### Exact live owner factual review

For an acknowledged page click/fill/type, Core can make one ordinary journaled
`browser.page_freeze_snapshot` revision1 read. Its ref is taken only from the
mediated, durably recorded post-effect result, not an active tab, original stale
selector or model-selected URL. It has its own finite slot/grant/budget and a
clipped five-second deadline. An approval requirement, changed authority,
missing result/ref or resource quarantine prevents the probe. Its durable
checkpoint prevents a second probe even after crash/error. It freezes the
whole context's connections/routes before observing the exact page.

Migration105's existing review/interaction kernel joins the source and frozen
read's immutable runtime-response digests, owner/Run/Task/authority versions,
page ID, post-effect document, PG versions and context epoch. A navigation
caused by the acknowledged action may produce a new document on that same page;
the frozen read must bind that returned document exactly. A genuine frozen
read of another page, ordinary snapshot, fabricated body or stale epoch cannot
satisfy the join. The owning context/page/current policy is checked at request
and report time. The owner card identifies safe URL plus page/document IDs.

The existing protected owner report requires recipient-checked and settled
assertions plus a receipt reference for confirmed/absent. Only digests persist.
Unknown stays UNKNOWN; known evidence is labelled authenticated owner
attestation, never a machine receipt. Canonical same-Input recovery rechecks its
claim/lease, source/current Task/mandate and all other uncertainty before model
admission. Original UNKNOWN transcript, consumed grant, budgets and resource
history remain. Reporting does not itself finish a task, deliver a response or
retry POST. No new migration, secret reference or broader approval is introduced.

This stage excludes unacknowledged/inflight page mutations, unavailable or
closed post-effect documents, page-close outcomes and historical managed-page
review. Those remain restrictive; they cannot borrow legacy login historical
maintenance or invent absence from runtime loss.

Legacy credential requests against managed contexts are denied; password fields
cannot accept ordinary literals. Migration109 adds a separate protected
[RP/IdP consent protocol](browser-sso-protocol.md). Migration110's separate
`browser.sso_login` is the only credential-aware managed transition: one
conditionally fresh IdP child, exact native form POST, freeze/cleanup and
Go-verified RP access with an anonymous negative control. Generic page tools
retain the credential-free same-origin boundary. Composed kernel execution,
native model delivery and the owner's dedicated Google account remain required
acceptance. Do not reuse provider control-plane tokens for Google.

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
