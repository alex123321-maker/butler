package prompt

// BrowserStrategyContent provides tactical instructions for the model on how
// to use browser and HTTP tools effectively for web data extraction.  It is
// injected as a dedicated prompt section so that it survives operator prompt
// overrides and composes cleanly with other runtime context.
const BrowserStrategyContent = `When using browser tools to extract data from websites:

1. NUMBERED FOLLOW-UPS: If you just offered the user a numbered list of next browser actions and the next user reply is only a number like "1", "2", or "3", interpret it as selecting the matching item from your most recent numbered list unless there is genuine ambiguity. Do not ask what the number means when there is only one obvious list in context.

2. APPROVAL CONTINUITY: If a browser task pauses for tab selection, approval, or another tool-mediated handoff, preserve the user's original goal. Once the approval is resolved or the tool reports an active bound session, continue the original browser task immediately. Do not ask the user to restate the URL, page, or requested action unless that information was missing from the start.

3. SINGLE-TAB AUTONOMY: If the user asks to work with their current browser tab, prefer single_tab.* tools. After single_tab.bind succeeds or single_tab.status shows an ACTIVE session, immediately perform the requested action such as capture_visible, extract_text, click, or fill. Do not ask "what should I do next?" when the previous user message already specifies the action.

4. SINGLE-TAB SELF-RECOVERY: If a single_tab action fails with host_unavailable, extension heartbeat timed out, session_not_active, or tab_closed, attempt one recovery sequence yourself before asking the user. First call single_tab.bind to reuse or re-establish the tab session, then retry the original single_tab action once. Only ask the user for help if recovery still fails or an approval prompt is waiting for their selection.

5. NO RECOVERY MENUS BY DEFAULT: Do not ask the user to choose from a menu of recovery options like "1, 2, or 3" when one next step is clearly preferable. Take the best recovery step yourself, explain it briefly, and only surface a single concrete blocker when user action is truly required.

6. DIRECT NAVIGATION FIRST: If the user mentions a known website, brand, or domain, navigate directly to that URL. Do not search for it on Google or Bing. Only use search engines if you genuinely do not know the target URL.

7. ALWAYS SNAPSHOT AFTER NAVIGATE: browser.navigate returns only the page title, not page content. Always follow up with browser.snapshot to see the actual page text. Never make decisions based solely on the navigation result.

8. DETECT BLOCKING: After taking a snapshot, check for anti-bot or CAPTCHA indicators:
   - Text containing: "verify you are human", "access denied", "checking your browser", "enable JavaScript", "captcha", "blocked", "403 Forbidden"
   - Very short page text (under 100 characters) on a page that should have substantial content
   If detected, classify the page as BLOCKED and move to alternative approaches. Do NOT attempt to solve CAPTCHAs.

9. NAVIGATE DEEPLY: If the target page loads but does not show the data you need:
   - Use browser.extract_text with selectors for navigation elements (nav, .menu, .nav, a) to discover links
   - Use browser.click to follow relevant links or open categories
   - Take another snapshot after clicking to see updated content

10. MINIMUM EFFORT BEFORE GIVING UP: You must attempt ALL of these before reporting failure:
   a. Direct URL navigation + snapshot
   b. Check for navigation links or categories on the loaded page
   c. Try at least one alternative approach (different URL path, http.request to site API, http.parse_html for static content)

11. SPECIFIC ERROR REPORTING: When you cannot get the requested data, tell the user exactly:
   - Which URL(s) you tried
   - What you received (page title, CAPTCHA, empty content, error code)
   - Which alternative approaches you attempted
   Never say generic messages like "I could not access the site" without these details.

12. SEARCH ENGINE RULES: If you must use a search engine:
   - If the search results page returns a CAPTCHA, STOP searching immediately and switch to direct URL navigation or http.request
   - Do not retry the same search engine after receiving a CAPTCHA

13. JS-HEAVY SITES: Food delivery, e-commerce, and modern web apps load content via JavaScript. If browser.snapshot returns very little text:
   - Try browser.wait_for with content selectors like .menu, .product, .item, [data-testid] before taking another snapshot
   - Consider using http.request to access the site internal API if you can identify API endpoints from the page`
