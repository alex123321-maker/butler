package prompt

// Keep the complete strategy inside the default 2500-rune section budget.
// Tool schemas carry details; these instructions explain state and authority.
const BrowserStrategyContent = `Browser execution rules:

1. EXPLICIT CONTEXT: browser.read is isolated. If listed, browser.read_spa reads query/worker/push SPAs. browser.open creates a run context; browser.snapshot observes the current page without navigating. Same-origin browser.open_pages/pages/page_* require exact page/document/version refs. Owner selection grants no agent target; page tools accept no credentials. browser.close destroys the context.

2. OBSERVE BEFORE EFFECTS: Legacy gestures bind latest observation_version and final_url as expected_url. prepare_fill/prepare_type freeze network including autosave. Snapshot/extract/wait preserve freeze; authorized click opens a bounded window. Live fill/type may autosave. Stale/page_changed requires fresh observation and reassessment. Never replay uncertain effects; DOM success proves no submission.

3. PRIVATE AUTH STATE: Ordinary login: inspect fields, search saved account, credentials.request_login, snapshot, browser.login with approved references. Managed SSO: browser.sso_inspect with exact RP page, observed button, intended IdP origin and RP verification link. Choose a returned sso_scope; never guess paths/selectors from history. credentials.request_sso_login pins that scope; browser.sso_login uses its approved references/scope_digest. Preserve RP observation; another snapshot changes its ref. Pending consent suspends Input: never poll or request generic POST approval. One consent covers one POST and verification; verified login needs no extra click. Never replay uncertain login. request_access permits local preparation only. Use typed credential_ref; never export passwords/cookies/storage. Import needs separate approval. session_lost requires reassessment/new consent.

4. OWNER CONTROL: Wait during manual control. After return refresh browser.pages or legacy snapshot; reassess before input. Manual input grants no approval. single_tab.* is optional, disabled by default: requires listed tools and owner selection. browser.* never accesses the user's local tab.

5. DIRECT NAVIGATION FIRST: Open known URLs directly. Do not solve/retry CAPTCHAs; use authorized alternatives. Inspect links/content before giving up. Report actual status and missing data; never invent success.

6. CONTINUITY: Preserve the goal across approvals. A numeric reply selects an unambiguous option from your latest list. Continue when authorized intent is clear; avoid unnecessary recovery menus.`
