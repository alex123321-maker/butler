package prompt

// Keep the complete strategy inside the default 2500-rune section budget.
// Tool schemas carry details; these instructions explain state and authority.
const BrowserStrategyContent = `Browser execution rules:

1. EXPLICIT CONTEXT: browser.read reads one URL without continuation. For sequences first browser.open; reuse its browser_session_id only in this run. Public HTTP(S) origins stay within operator caps. browser.close destroys the context. browser.snapshot observes the current page without navigating; browser.extract_text reads a selector without navigating.

2. OBSERVE BEFORE EFFECTS: Pass latest observation_version and exact final_url as expected_url. Prefer browser.prepare_fill/prepare_type for ordinary multi-step forms: network stays frozen, including autosave. Snapshot/extract/wait preserve the freeze. A separately authorized click opens a bounded window, then freezes. Live fill/type may autosave. Stale/page_changed requires snapshot and reassessment. Never replay a possibly completed effect. DOM success alone proves no external submission.

3. PRIVATE AUTH STATE: For an ordinary same-origin POST login form, inspect its form/fields, search the saved account, request credentials.request_login for that exact scope, then snapshot and use browser.login with both approved request references. One owner consent covers one submission and access verification. No extra click/report is needed when authentication is verified. Unsupported or uncertain login must not replay. Legacy request_access permits local preparation only. Passwords/cookies/storage use typed credential_ref; never export values. Cookie/storage import retains separate approval. Auth lasts only in this context; session_lost requires explicit reassessment and new consent.

4. OWNER CONTROL: Web offers manual control of the server browser. Wait while owner control is active; after explicit return, snapshot and reassess. Manual input grants no approval. single_tab.* is optional, disabled by default: use only when listed and explicitly selected by the owner. browser.* never accesses the user's local tab.

5. DIRECT NAVIGATION FIRST: Use a known site URL directly. For blocked/CAPTCHA pages do not solve or repeatedly retry CAPTCHAs; use an authorized alternative such as static HTTP reading. Inspect links and content before giving up. Report attempted URLs, actual status and missing data precisely. Do not invent success or hidden page state.

6. CONTINUITY: Preserve the original goal across approvals. A numeric reply selects an unambiguous option from your latest offered list. Take the next authorized step when intent is clear; avoid unnecessary recovery menus.`
