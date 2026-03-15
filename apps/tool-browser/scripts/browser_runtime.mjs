import { chromium } from 'playwright'

const raw = process.argv[2]
if (!raw) {
  console.error('missing browser request payload')
  process.exit(1)
}

const request = JSON.parse(raw)
const browser = await chromium.launch({ headless: true })

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
    await page.goto(request.url, { waitUntil: 'domcontentloaded', timeout: 15000 })
  }

  const toolName = request.tool_name
  const timeout = request.timeout || 5000

  if (toolName === 'browser.navigate') {
    const title = await page.title()
    const finalURL = page.url()
    process.stdout.write(JSON.stringify({ final_url: finalURL, title, text: '' }))

  } else if (toolName === 'browser.snapshot') {
    const title = await page.title()
    const finalURL = page.url()
    const text = await page.locator('body').innerText().catch(() => '')
    process.stdout.write(JSON.stringify({ final_url: finalURL, title, text }))

  } else if (toolName === 'browser.click') {
    await page.locator(request.selector).click({ timeout })
    // Wait briefly for any navigation or re-render.
    await page.waitForLoadState('domcontentloaded').catch(() => {})
    const title = await page.title()
    process.stdout.write(JSON.stringify({ ok: true, title }))

  } else if (toolName === 'browser.fill') {
    await page.locator(request.selector).fill(request.value || '', { timeout })
    process.stdout.write(JSON.stringify({ ok: true }))

  } else if (toolName === 'browser.type') {
    await page.locator(request.selector).pressSequentially(request.text || '', { timeout, delay: 20 })
    process.stdout.write(JSON.stringify({ ok: true }))

  } else if (toolName === 'browser.wait_for') {
    try {
      await page.locator(request.selector).waitFor({ state: 'visible', timeout })
      process.stdout.write(JSON.stringify({ matched: true }))
    } catch {
      process.stdout.write(JSON.stringify({ matched: false }))
    }

  } else if (toolName === 'browser.extract_text') {
    const text = await page.locator(request.selector).innerText({ timeout }).catch(() => '')
    process.stdout.write(JSON.stringify({ text }))

  } else {
    console.error(`unsupported tool: ${toolName}`)
    process.exit(1)
  }

} finally {
  await browser.close()
}
