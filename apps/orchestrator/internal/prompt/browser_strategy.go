package prompt

// Keep the complete strategy inside the default 2500-rune section budget.
// Tool schemas carry details; these instructions explain state and authority.
const BrowserStrategyContent = `Browser execution rules:

1. EXPLICIT CONTEXT: browser.read reads one URL in a temporary context and creates no continuation. For a sequence first browser.open. Reuse its browser_session_id only in this run and fixed scope_origin. browser.close destroys the context. browser.snapshot observes the current page without navigating; browser.extract_text reads a selector without navigating.

2. OBSERVE BEFORE EFFECTS: Pass latest observation_version and exact final_url as expected_url for actions. Prefer browser.prepare_fill/prepare_type for multi-step forms: all network stays frozen, including delayed autosave. Snapshot/extract/wait keep that freeze. Only a separately authorized click opens a bounded window, then freezes again. Live fill/type may autosave and have an unknown external outcome; use them only when intended. A frozen draft cannot navigate. Stale/page_changed requires snapshot and reassessment. Never replay a possibly completed effect. DOM success does not prove an external submission.

3. PRIVATE AUTH STATE: Password/token fields, cookie and storage_state use typed credential_ref. Cookie/storage injection retains its separate approval. Never pass or export raw passwords, cookies or storage. Auth persists only in that live context. session_expired/session_lost means state is gone; report loss and decide explicitly before reopening. No transfer to another run or silent crash recovery.

4. USER TAB: For the user's current tab prefer single_tab.*. Bind requires the user's explicit tab selection; do not list/switch/open/focus tabs or enlarge access. After approval/bind continue the original goal without asking the user to restate it. On unavailable host/tab, attempt one bind recovery and one retry, respecting any new selection approval. Ask for help only if recovery fails or approval awaits the user. browser.* never grants access to a user tab.

5. DIRECT NAVIGATION FIRST: Use a known site URL directly. For blocked/CAPTCHA pages do not solve or repeatedly retry CAPTCHAs; use an authorized alternative such as static HTTP reading. Inspect links and content before giving up. Report attempted URLs, actual status and missing data precisely. Do not invent success or hidden page state.

6. CONTINUITY: Preserve the original goal across approvals. A numeric reply selects an unambiguous option from your latest offered list. Take the next authorized step when intent is clear; avoid unnecessary recovery menus.`
