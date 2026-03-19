package prompt

// BrowserStrategyContent provides tactical instructions for the model on how
// to use browser and HTTP tools effectively for web data extraction.  It is
// injected as a dedicated prompt section so that it survives operator prompt
// overrides and composes cleanly with other runtime context.
const BrowserStrategyContent = `When using browser tools to extract data from websites:

1. DIRECT NAVIGATION FIRST: If the user mentions a known website, brand, or domain, navigate directly to that URL. Do not search for it on Google or Bing. Only use search engines if you genuinely do not know the target URL.

2. ALWAYS SNAPSHOT AFTER NAVIGATE: browser.navigate returns only the page title, not page content. Always follow up with browser.snapshot to see the actual page text. Never make decisions based solely on the navigation result.

3. DETECT BLOCKING: After taking a snapshot, check for anti-bot or CAPTCHA indicators:
   - Text containing: "verify you are human", "access denied", "checking your browser", "enable JavaScript", "captcha", "blocked", "403 Forbidden"
   - Very short page text (under 100 characters) on a page that should have substantial content
   If detected, classify the page as BLOCKED and move to alternative approaches. Do NOT attempt to solve CAPTCHAs.

4. NAVIGATE DEEPLY: If the target page loads but does not show the data you need:
   - Use browser.extract_text with selectors for navigation elements (nav, .menu, .nav, a) to discover links
   - Use browser.click to follow relevant links or open categories
   - Take another snapshot after clicking to see updated content

5. MINIMUM EFFORT BEFORE GIVING UP: You must attempt ALL of these before reporting failure:
   a. Direct URL navigation + snapshot
   b. Check for navigation links or categories on the loaded page
   c. Try at least one alternative approach (different URL path, http.request to site API, http.parse_html for static content)

6. SPECIFIC ERROR REPORTING: When you cannot get the requested data, tell the user exactly:
   - Which URL(s) you tried
   - What you received (page title, CAPTCHA, empty content, error code)
   - Which alternative approaches you attempted
   Never say generic messages like "I could not access the site" without these details.

7. SEARCH ENGINE RULES: If you must use a search engine:
   - If the search results page returns a CAPTCHA, STOP searching immediately and switch to direct URL navigation or http.request
   - Do not retry the same search engine after receiving a CAPTCHA

8. JS-HEAVY SITES: Food delivery, e-commerce, and modern web apps load content via JavaScript. If browser.snapshot returns very little text:
   - Try browser.wait_for with content selectors like .menu, .product, .item, [data-testid] before taking another snapshot
   - Consider using http.request to access the site internal API if you can identify API endpoints from the page`
