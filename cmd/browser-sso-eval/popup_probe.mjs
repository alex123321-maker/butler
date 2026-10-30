// Standalone fixture acceptance only. This is not a Butler tool/runtime and
// cannot confer browser/credential authority. Private configuration is stdin.
import { chromium } from 'playwright'
import { createInterface } from 'node:readline'

let browser, context
let result = { scope: 'standalone_sso_fixture', ok: false, model_calls: 0 }
let stage = 'setup'
try {
  const lines = createInterface({ input: process.stdin, terminal: false })
  const raw = await new Promise(resolve => lines.once('line', resolve))
  lines.close()
  if (raw.length > 16384) throw new Error('invalid private input')
  const { rp, idp, username, password, private_fact, mode } = JSON.parse(raw)
  if (!['normal', 'late', 'twins', 'closed_before_login', 'forged_message'].includes(mode)) throw new Error('invalid mode')
  const origins = new Set([new URL(rp).origin, new URL(idp).origin])
  const privateValues = new Set([username, password, private_fact])
  browser = await chromium.launch({ headless: true, args: ['--no-proxy-server', '--disable-quic', '--disable-background-networking'] })
  context = await browser.newContext({ serviceWorkers: 'block', acceptDownloads: false })
  await context.route('**/*', route => origins.has(new URL(route.request().url()).origin) ? route.continue() : route.abort())
  await context.routeWebSocket('**/*', socket => socket.close())
  const pages = []
  context.on('page', page => {
    pages.push(page)
    page.on('response', response => {
      const url = new URL(response.url())
      if (url.origin === idp && url.pathname === '/login') result.login_status = response.status()
      if (url.origin === rp && url.pathname === '/callback') result.callback_status = response.status()
      if (url.origin === rp && url.pathname === '/account') result.account_status = response.status()
    })
    page.on('framenavigated', frame => {
      if (frame !== page.mainFrame()) return
      try {
        const url = new URL(frame.url())
        for (const value of url.searchParams.values()) if (value.length >= 8) privateValues.add(value)
        if (url.hash.length >= 8) privateValues.add(url.hash.slice(1))
      } catch {}
    })
  })
  const opener = await context.newPage()
  await opener.goto(rp + '/', { waitUntil: 'domcontentloaded', timeout: 8000 })
  if (mode === 'twins') {
    const second = context.waitForEvent('page', { predicate: () => pages.length === 3, timeout: 8000 })
    await opener.locator('#twins').click()
    await second
    const twins = pages.filter(page => page !== opener)
    await Promise.all(twins.map(page => page.waitForURL(url => url.origin === idp, { timeout: 8000 })))
    await Promise.all(twins.map(page => page.waitForLoadState('domcontentloaded')))
    result.identical_titles_distinct_pages = twins.length === 2 && twins[0] !== twins[1] && (await twins[0].title()) === (await twins[1].title())
    result.ok = result.identical_titles_distinct_pages
  } else {
    const created = context.waitForEvent('page', { timeout: 8000 })
    await opener.locator(mode === 'late' ? '#late' : '#login').click()
    const popup = await created
    await popup.waitForURL(url => url.origin === idp, { timeout: 8000 })
    result.opener_linked = (await popup.opener()) === opener
    result.cross_origin_sop = await popup.evaluate(() => { try { return !opener.document } catch { return true } })
    if (mode === 'closed_before_login' || mode === 'forged_message') {
      if (mode === 'closed_before_login') await popup.close()
      else await opener.evaluate(() => window.postMessage({ kind: 'sso_complete' }, location.origin))
      const response = await context.request.get(rp + '/account', { maxRedirects: 0 })
      result.close_or_message_is_not_login = response.status() === 401 && opener.url() === rp + '/'
      result.ok = result.opener_linked && result.cross_origin_sop && result.close_or_message_is_not_login
    } else {
      stage = 'fill'
      await popup.locator('input[name=username]').fill(username)
      await popup.locator('input[name=password]').fill(password)
      stage = 'submit_return'
      await Promise.all([
        popup.waitForEvent('close', { timeout: 8000 }),
        opener.waitForURL(rp + '/account', { timeout: 8000 }),
        popup.locator('button').click({ noWaitAfter: true }),
      ])
      stage = 'private_read'
      result.private_fact_matches = (await opener.locator('#private-ref').innerText()) === private_fact
      stage = 'anonymous_read'
      const anonymous = await browser.newContext()
      await anonymous.route('**/*', route => origins.has(new URL(route.request().url()).origin) ? route.continue() : route.abort())
      const challenged = await anonymous.request.get(rp + '/account', { maxRedirects: 0 })
      result.anonymous_challenged = challenged.status() === 401
      await anonymous.close()
      result.popup_closed = popup.isClosed()
      result.ok = result.opener_linked && result.cross_origin_sop && result.private_fact_matches && result.anonymous_challenged && result.popup_closed
    }
  }
  for (const cookie of await context.cookies()) if (cookie.value) privateValues.add(cookie.value)
  result.private_material_exported = [...privateValues].some(value => value && JSON.stringify(result).includes(value))
  result.ok = result.ok && !result.private_material_exported
} catch {
  result.ok = false // Never serialize URLs, codes, cookies or Playwright errors.
  result.failure_stage = stage
} finally {
  await context?.close().catch(() => {})
  await browser?.close().catch(() => {})
}
process.stdout.write(JSON.stringify(result) + '\n')
