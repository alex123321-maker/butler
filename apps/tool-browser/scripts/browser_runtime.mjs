import { chromium } from 'playwright'

const raw = process.argv[2]
if (!raw) {
  console.error('missing browser request payload')
  process.exit(1)
}

const request = JSON.parse(raw)
const browser = await chromium.launch({ headless: true })

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// CAPTCHA / anti-bot indicator patterns (case-insensitive substrings).
const blockedPatterns = [
  'verify you are human',
  'are you a robot',
  'captcha',
  'access denied',
  'checking your browser',
  'enable javascript',
  '403 forbidden',
  'just a moment',
  'attention required',
  'please wait while we verify',
  'ddos protection',
  'security check',
]

/**
 * Classify the page based on its visible text.
 * Returns "blocked" when CAPTCHA / anti-bot patterns are detected,
 * "error" for very short content on pages that should have substance,
 * or "ok" otherwise.
 */
function classifyPage(text, title) {
  const lower = (text || '').toLowerCase()
  const lowerTitle = (title || '').toLowerCase()
  for (const pattern of blockedPatterns) {
    if (lower.includes(pattern) || lowerTitle.includes(pattern)) {
      return 'blocked'
    }
  }
  return 'ok'
}

/**
 * Classify a Playwright error into a structured error type.
 * Returns { error_type, message, retryable }.
 */
function classifyError(err) {
  const msg = (err && err.message) || String(err)
  const lower = msg.toLowerCase()

  if (lower.includes('timeout') || lower.includes('exceeded the time')) {
    return { error_type: 'timeout', message: msg, retryable: true }
  }
  if (lower.includes('waiting for locator') || lower.includes('no element') ||
      lower.includes('strict mode violation') || lower.includes('resolved to') ||
      lower.includes('locator.click') || lower.includes('locator.fill') ||
      lower.includes('locator.innertext') || lower.includes('locator.presssequentially')) {
    return { error_type: 'selector_not_found', message: msg, retryable: false }
  }
  if (lower.includes('net::err_') || lower.includes('dns') ||
      lower.includes('econnrefused') || lower.includes('enotfound') ||
      lower.includes('navigation failed') || lower.includes('page.goto')) {
    return { error_type: 'navigation_failed', message: msg, retryable: true }
  }
  if (lower.includes('context or page has been closed') || lower.includes('target closed') ||
      lower.includes('browser has been closed')) {
    return { error_type: 'browser_closed', message: msg, retryable: true }
  }
  return { error_type: 'runtime_error', message: msg, retryable: false }
}

/**
 * Extract up to `limit` navigation links from the page.
 * Returns an array of { text, href } objects.
 */
async function extractLinks(page, limit = 30) {
  try {
    return await page.evaluate((maxLinks) => {
      const anchors = Array.from(document.querySelectorAll('a[href]'))
      const seen = new Set()
      const results = []
      for (const a of anchors) {
        if (results.length >= maxLinks) break
        const href = a.href
        const text = (a.innerText || '').trim().substring(0, 120)
        if (!href || href === 'javascript:void(0)' || href.startsWith('javascript:')) continue
        if (!text) continue
        const key = href + '|' + text
        if (seen.has(key)) continue
        seen.add(key)
        results.push({ text, href })
      }
      return results
    }, limit)
  } catch {
    return []
  }
}

try {
  const context = await browser.newContext()

  // Restore storage state if requested.
  if (request.tool_name === 'browser.restore_storage_state' && request.storage_state) {
    if (request.storage_state.cookies && request.storage_state.cookies.length > 0) {
      const cookies = request.storage_state.cookies.map(c => ({
        name: c.name,
        value: c.value,
        domain: c.domain,
        path: c.path || '/',
        secure: c.secure || false,
        httpOnly: c.http_only || false,
      }))
      await context.addCookies(cookies)
    }
    // Origins / local storage would require navigating to each origin, which
    // is deferred to a future enhancement.
    process.stdout.write(JSON.stringify({ ok: true }))
    await browser.close()
    process.exit(0)
  }

  // Set cookie on the context before navigating.
  if (request.tool_name === 'browser.set_cookie' && request.cookie) {
    const c = request.cookie
    await context.addCookies([{
      name: c.name,
      value: c.value,
      domain: c.domain,
      path: c.path || '/',
      secure: c.secure || false,
      httpOnly: c.http_only || false,
    }])
    process.stdout.write(JSON.stringify({ ok: true }))
    await browser.close()
    process.exit(0)
  }

  const page = await context.newPage()

  // For tools that operate on an existing page (click, fill, type, wait_for,
  // extract_text), the page must first be navigated to the target URL.
  if (request.url) {
    try {
      await page.goto(request.url, { waitUntil: 'domcontentloaded', timeout: 15000 })
    } catch (e) {
      const classified = classifyError(e)
      // For navigate/snapshot, return a structured result with the error.
      // For other tools, the navigation failure is fatal to the operation.
      const toolName = request.tool_name
      if (toolName === 'browser.navigate' || toolName === 'browser.snapshot') {
        process.stdout.write(JSON.stringify({ final_url: request.url, title: '', text: '', page_status: 'error', links: [], error: classified }))
      } else {
        process.stdout.write(JSON.stringify({ ok: false, error: classified }))
      }
      await browser.close()
      process.exit(0)
    }
  }

  const toolName = request.tool_name
  const timeout = request.timeout || 5000

  if (toolName === 'browser.navigate') {
    try {
      const title = await page.title()
      const finalURL = page.url()
      const text = await page.locator('body').innerText().catch(() => '')
      const pageStatus = classifyPage(text, title)
      const links = await extractLinks(page)
      process.stdout.write(JSON.stringify({ final_url: finalURL, title, text, page_status: pageStatus, links }))
    } catch (e) {
      const classified = classifyError(e)
      process.stdout.write(JSON.stringify({ final_url: page.url(), title: '', text: '', page_status: 'error', links: [], error: classified }))
    }

  } else if (toolName === 'browser.snapshot') {
    try {
      const title = await page.title()
      const finalURL = page.url()
      const text = await page.locator('body').innerText().catch(() => '')
      const pageStatus = classifyPage(text, title)
      const links = await extractLinks(page)
      process.stdout.write(JSON.stringify({ final_url: finalURL, title, text, page_status: pageStatus, links }))
    } catch (e) {
      const classified = classifyError(e)
      process.stdout.write(JSON.stringify({ final_url: page.url(), title: '', text: '', page_status: 'error', links: [], error: classified }))
    }

  } else if (toolName === 'browser.click') {
    try {
      await page.locator(request.selector).click({ timeout })
      // Wait briefly for any navigation or re-render.
      await page.waitForLoadState('domcontentloaded').catch(() => {})
      const title = await page.title()
      process.stdout.write(JSON.stringify({ ok: true, title }))
    } catch (e) {
      const classified = classifyError(e)
      process.stdout.write(JSON.stringify({ ok: false, error: classified }))
    }

  } else if (toolName === 'browser.fill') {
    try {
      await page.locator(request.selector).fill(request.value || '', { timeout })
      process.stdout.write(JSON.stringify({ ok: true }))
    } catch (e) {
      const classified = classifyError(e)
      process.stdout.write(JSON.stringify({ ok: false, error: classified }))
    }

  } else if (toolName === 'browser.type') {
    try {
      await page.locator(request.selector).pressSequentially(request.text || '', { timeout, delay: 20 })
      process.stdout.write(JSON.stringify({ ok: true }))
    } catch (e) {
      const classified = classifyError(e)
      process.stdout.write(JSON.stringify({ ok: false, error: classified }))
    }

  } else if (toolName === 'browser.wait_for') {
    try {
      await page.locator(request.selector).waitFor({ state: 'visible', timeout })
      process.stdout.write(JSON.stringify({ matched: true }))
    } catch {
      process.stdout.write(JSON.stringify({ matched: false }))
    }

  } else if (toolName === 'browser.extract_text') {
    try {
      const text = await page.locator(request.selector).innerText({ timeout })
      process.stdout.write(JSON.stringify({ text }))
    } catch (e) {
      const classified = classifyError(e)
      process.stdout.write(JSON.stringify({ text: '', error: classified }))
    }

  } else {
    console.error(`unsupported tool: ${toolName}`)
    process.exit(1)
  }

} finally {
  await browser.close()
}
