package prompt

// Keep the complete strategy inside the default 2500-rune section budget.
// Tool schemas carry details; these instructions explain state and authority.
const BrowserStrategyContent = `Browser execution rules:

1. EXPLICIT CONTEXT: browser.read is isolated; browser.open creates a legacy single-page run context. browser.snapshot observes the current page without navigating. For same-origin tabs use browser.open_pages, browser.pages and browser.page_* with exact returned page/document/version refs. Owner selection grants no agent target. Page tools support no credentials or cross-origin SSO. browser.close destroys the context.

2. OBSERVE BEFORE EFFECTS: Legacy gestures bind latest observation_version and final_url as expected_url. prepare_fill/prepare_type stage ordinary fields with network frozen, including autosave. Snapshot/extract/wait preserve freeze; an authorized click opens a bounded window. Live fill/type may autosave. Stale/page_changed requires fresh observation and reassessment. Never replay an uncertain effect; DOM success proves no external submission.

3. PRIVATE AUTH STATE: For legacy same-origin form login inspect fields, search the saved account, request credentials.request_login for that exact scope, then snapshot and browser.login with both approved references. One consent covers one submission and access verification; verified login needs no extra click/report. Never replay uncertain login. request_access permits local preparation only. Use typed credential_ref; never export passwords/cookies/storage. Cookie/storage import needs separate approval. session_lost requires reassessment and new consent.

4. OWNER CONTROL: Wait during manual control. After return refresh browser.pages for managed tabs, or snapshot for legacy; reassess before input. Manual input grants no approval. single_tab.* is optional, disabled by default: use only when listed and owner-selected. browser.* never accesses the user's local tab.

5. DIRECT NAVIGATION FIRST: Open known URLs directly. Do not solve or repeatedly retry CAPTCHAs; use an authorized alternative such as static HTTP. Inspect links/content before giving up. Report attempted URLs, actual status and missing data; never invent success or hidden state.

6. CONTINUITY: Preserve the goal across approvals. A numeric reply selects an unambiguous option from your latest offered list. Continue when authorized intent is clear; avoid unnecessary recovery menus.`
