import { chromium } from 'playwright'
import { createInterface } from 'node:readline'

// Private JSON-lines protocol. Never print page diagnostics, request bodies,
// cookies, storage state, passwords or Playwright exceptions.
const sessions = new Map()
let browser

const errorResult = (error_type, message) => ({ ok: false, error: { error_type, message, retryable: false } })
const origin = value => { try { return new URL(value).origin } catch { return '' } }
const safeError = err => {
  const message = String(err?.message || '')
  if (message.toLowerCase().includes('timeout')) return errorResult('timeout', 'browser operation timed out; observe before another action')
  if (message.toLowerCase().includes('closed')) return errorResult('session_lost', 'browser context was lost; explicitly open a new session')
  return errorResult('operation_failed', 'browser operation failed; observe before another action')
}

async function closeSession(id) {
  const session = sessions.get(id)
  if (!session) return
  sessions.delete(id)
  clearTimeout(session.expiryTimer)
  await session.context.close().catch(() => {})
  session.secrets.clear()
}

async function ensureBrowser() {
  if (browser?.isConnected()) return browser
  browser = await chromium.launch({ headless: true, args: ['--disable-quic', '--disable-background-networking', '--force-webrtc-ip-handling-policy=disable_non_proxied_udp', '--proxy-bypass-list=<-loopback>'] })
  browser.on('disconnected', () => {
    for (const session of sessions.values()) { clearTimeout(session.expiryTimer); session.secrets.clear() }
    sessions.clear()
  })
  return browser
}

function redact(session, value) {
  let text = String(value ?? '')
  const variants = new Set()
  for (const secret of session.secrets) {
    if (!secret) continue
    const b64 = Buffer.from(secret).toString('base64')
    for (const variant of [secret, encodeURIComponent(secret), encodeURIComponent(secret).replaceAll('%20', '+'), b64, b64.replace(/=+$/, ''), Buffer.from(secret).toString('base64url')]) variants.add(variant)
  }
  // Match against the original text, then mask the union. Keyboard entry
  // collects successive prefixes: replacing a prefix first would hide the
  // longer match and expose its suffix. Overlapping secrets must merge too.
  if (variants.size) {
    const masked = new Uint8Array(text.length)
    for (const variant of variants) {
      for (let start = text.indexOf(variant); start >= 0; start = text.indexOf(variant, start + 1)) masked.fill(1, start, start + variant.length)
    }
    const parts = []
    let cursor = 0, index = 0
    while (index < text.length) {
      if (!masked[index]) { index++; continue }
      parts.push(text.slice(cursor, index), '[REDACTED]')
      while (masked[index]) index++
      cursor = index
    }
    parts.push(text.slice(cursor))
    text = parts.join('')
  }
  return text.replace(/([?&#](?:access_token|refresh_token|token|password|api_key|secret|code)=)[^&#]*/gi, '$1[REDACTED]')
}

async function collectSecretState(session) {
  // Values stay exclusively in process memory. Page-derived output is masked
  // against auth data even when a page reflects a cookie or local storage value.
  for (const cookie of await session.context.cookies()) if (cookie.value) session.secrets.add(cookie.value)
  for (const frame of session.page.frames()) {
  const values = await frame.evaluate(() => {
    const values = []
    for (const input of document.querySelectorAll('input[type=password]')) if (input.value) values.push(input.value)
    try { for (let i = 0; i < localStorage.length; i++) values.push(localStorage.getItem(localStorage.key(i))) } catch {}
    try { for (let i = 0; i < sessionStorage.length; i++) values.push(sessionStorage.getItem(sessionStorage.key(i))) } catch {}
    return values
  }).catch(() => [])
  for (const value of values) if (value) session.secrets.add(value)
  }
}

async function newSession(request) {
  const browser = await ensureBrowser()
  if (!request.proxy_url) throw new Error('private egress proxy missing')
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 }, serviceWorkers: 'block', acceptDownloads: false, proxy: { server: request.proxy_url, bypass: '' } })
  const session = { context, page: undefined, scope: request.scope_origin || origin(request.url),
    version: 0, dirty: true, secrets: new Set(), frames: new Map(), allowedDomains: request.allowed_domains || [], expires: Date.parse(request.expires_at), expiryTimer: undefined, allowWriteRequests: false, networkFrozen: false }
  // Native same-origin and cookie rules remain enabled. The private proxy
  // checks public DNS on every connection; this route retains contract caps.
  await context.route('**/*', async route => {
    const url = new URL(route.request().url())
    const allowed = ['http:', 'https:'].includes(url.protocol) && !url.username && !url.password && (!session.allowedDomains.length || session.allowedDomains.some(domain => domain === '*' || url.hostname === domain || url.hostname.endsWith('.' + domain)))
    if (session.networkFrozen || !allowed || (!session.allowWriteRequests && !['GET', 'HEAD'].includes(route.request().method()))) await route.abort('blockedbyclient')
    else await route.continue()
  })
  await context.routeWebSocket('**/*', socket => socket.close())
  await context.addInitScript(() => {
    window.__butlerRevision = 0
    window.__butlerDocumentID = Array.from(crypto.getRandomValues(new Uint8Array(16)), value => value.toString(16).padStart(2, '0')).join('')
    const changed = () => { window.__butlerRevision++ }
    new MutationObserver(changed).observe(document, { subtree: true, childList: true, attributes: true, characterData: true })
    document.addEventListener('input', changed, true)
    document.addEventListener('change', changed, true)
    window.addEventListener('popstate', changed)
    window.addEventListener('hashchange', changed)
    window.addEventListener('scroll', changed, true)
  })
  const page = await context.newPage()
  session.page = page
  page.on('framenavigated', () => { session.dirty = true })
  context.on('page', other => { if (other !== page) void other.close() })
  return session
}

async function observe(session, selector = 'body') {
  await collectSecretState(session)
  // Read visible text only. Never serialize inputs, HTML attributes, cookies,
  // storage, screenshot pixels or full DOM into a normal tool artifact.
  // Capture text, URL and DOM revision in one JavaScript turn. A mutation
  // between separate reads must not validate an older text snapshot.
  const captured = await session.page.evaluate(selector => {
    const element = document.querySelector(selector)
    if (!element) throw new Error('requested selector is not present')
    const links = selector === 'body' ? Array.from(document.querySelectorAll('a[href]')).slice(0, 30)
      .map(a => ({ text: (a.innerText || '').slice(0, 120), href: a.href })).filter(a => a.text && /^https?:/.test(a.href)) : []
    return { text: element.innerText || '', links, title: document.title, url: location.href,
      revision: window.__butlerRevision, documentID: window.__butlerDocumentID }
  }, selector)
  await collectSecretState(session)
  const { text, links, title } = captured
  const final_url = redact(session, captured.url)
  session.observedRevision = captured.revision
  session.observedDocumentID = captured.documentID
  session.version++
  session.dirty = false
  const cleanText = Array.from(redact(session, text)).slice(0, 12000).join('')
  const page_status = /captcha|verify you are human|access denied|checking your browser|403 forbidden/i.test(cleanText + title) ? 'blocked' : 'ok'
  return { ok: true, observation_version: session.version, final_url, title: redact(session, title), text: cleanText, page_status, network_state: session.networkFrozen ? 'frozen' : 'read_only',
    links: links.map(link => ({ text: redact(session, link.text), href: redact(session, link.href) })) }
}

async function execute(request) {
  const tool = request.tool_name
  if (tool === 'browser.read') {
    const session = await newSession(request)
    try { await session.page.goto(request.url, { waitUntil: 'domcontentloaded', timeout: 15000 }); return await observe(session) }
    finally { await session.context.close(); session.secrets.clear() }
  }
  if (tool === 'browser.open') {
    if (!request.browser_session_id || sessions.has(request.browser_session_id)) return errorResult('invalid_session', 'browser session identifier is invalid')
    const session = await newSession(request)
    sessions.set(request.browser_session_id, session)
    session.expiryTimer = setTimeout(() => { void closeSession(request.browser_session_id) }, Math.max(0, session.expires - Date.now()))
    session.expiryTimer.unref()
    try {
      await session.page.goto(request.url, { waitUntil: 'domcontentloaded', timeout: 15000 })
      return await observe(session)
    } catch (err) { await closeSession(request.browser_session_id); return safeError(err) }
  }
  const session = sessions.get(request.browser_session_id)
  if (!session) return errorResult('session_lost', 'browser session was closed, expired, or lost; explicitly open a new session')
  if (Date.now() >= session.expires) { await closeSession(request.browser_session_id); return errorResult('session_expired', 'browser session expired') }
  if (tool === 'owner.control') return await ownerControl(session, request.owner_command)
  if (tool === 'browser.close') { await closeSession(request.browser_session_id); return { ok: true } }
  const localPreparation = ['browser.prepare_fill', 'browser.prepare_type'].includes(tool)
  // Freeze before reading the live guards or injecting any new value. A stale
  // preparation can fail safely without resuming background page requests.
  if (localPreparation) session.networkFrozen = true
  if (session.networkFrozen && ['browser.navigate', 'browser.fill', 'browser.type'].includes(tool)) return errorResult('network_frozen', 'prepared context permits local preparation, observation, close or separately authorized click only')
  session.scope = origin(session.page.url())
  if (tool === 'browser.snapshot') return await observe(session)
  if (tool === 'browser.extract_text') return await observe(session, request.selector)
  // Re-check live page state immediately before every effect. Go already
  // checks trusted ownership, exclusive control, and the last observed state.
  await collectSecretState(session)
  const live = await session.page.evaluate(() => ({ revision: window.__butlerRevision, documentID: window.__butlerDocumentID }))
  if (request.observation_version !== session.version || session.dirty || live.revision !== session.observedRevision || live.documentID !== session.observedDocumentID) return errorResult('stale_observation', 'page changed since observation; take a new snapshot')
  if (request.expected_url !== redact(session, session.page.url())) return errorResult('page_changed', 'current page does not match expected_url; take a new snapshot')
  for (const secret of request.secret_values || []) if (secret) session.secrets.add(secret)
  const timeout = request.timeout || 5000
  session.allowWriteRequests = ['browser.click', 'browser.fill', 'browser.type'].includes(tool)
  // The Go proxy opens a bounded click window, but page traffic remains frozen
  // until the exact live observation and URL guards above succeed.
  if (tool === 'browser.click') session.networkFrozen = false
  try {
    switch (tool) {
      case 'browser.navigate':
        await session.page.goto(request.url, { waitUntil: 'domcontentloaded', timeout }); break
      case 'browser.click':
        await session.page.locator(request.selector).click({ timeout });
        await session.page.waitForLoadState('domcontentloaded', { timeout }); break
      case 'browser.fill':
      case 'browser.type':
      case 'browser.prepare_fill':
      case 'browser.prepare_type': {
        const locator = session.page.locator(request.selector)
        const password = await locator.evaluate(element => element instanceof HTMLInputElement && element.type === 'password', null, { timeout })
        if (password && !request.secret_values?.length) return errorResult('credential_required', 'password inputs require a broker-resolved credential_ref')
        if (tool === 'browser.fill' || tool === 'browser.prepare_fill') await locator.fill(request.value || '', { timeout })
        else await locator.pressSequentially(request.text || '', { timeout })
        break
      }
      case 'browser.wait_for':
        await session.page.locator(request.selector).waitFor({ state: 'visible', timeout });
        return { ...await observe(session), matched: true }
      case 'browser.set_cookie': {
        const c = request.cookie
        if (!c || c.domain.replace(/^\./, '').toLowerCase() !== new URL(session.scope).hostname.toLowerCase()) return errorResult('scope_denied', 'cookie cannot expand browser origin')
        await session.context.addCookies([{ name: c.name, value: c.value, domain: new URL(session.scope).hostname, path: c.path || '/', secure: !!c.secure, httpOnly: !!c.http_only }]); break
      }
      case 'browser.restore_storage_state': {
        const state = request.storage_state
        if (!state) return errorResult('invalid_auth_state', 'storage state was not broker-resolved')
        for (const cookie of state.cookies || []) {
          if (cookie.domain.replace(/^\./, '').toLowerCase() !== new URL(session.scope).hostname.toLowerCase()) return errorResult('scope_denied', 'storage cookie cannot expand browser origin')
        }
        for (const entry of state.origins || []) if (origin(entry.origin) !== session.scope) return errorResult('scope_denied', 'storage origin cannot expand browser scope')
        await session.context.addCookies((state.cookies || []).map(c => ({ name: c.name, value: c.value, domain: new URL(session.scope).hostname, path: c.path || '/', secure: !!c.secure, httpOnly: !!c.http_only })))
        for (const entry of state.origins || []) await session.page.evaluate(entries => {
          for (const item of entries) localStorage.setItem(item.name, item.value)
        }, entry.local_storage || [])
        break
      }
      default: return errorResult('unsupported_tool', 'unsupported browser tool')
    }
    // A new observation is returned after effects, so fill A -> fill B -> click
    // can use each returned version without silently reloading the page.
    if (tool === 'browser.click') session.networkFrozen = true
    return { ...await observe(session), ...(localPreparation ? { local_preparation: true } : {}) }
  } catch (err) {
    session.version++
    session.dirty = true
    return { ...safeError(err), observation_version: session.version, final_url: redact(session, session.page.url()) }
  } finally {
    session.allowWriteRequests = false
    if (tool === 'browser.click' || localPreparation) session.networkFrozen = true
  }
}

async function ownerControl(session, command) {
  const page = session.page
  if (command.operation === 'release') {
    session.networkFrozen = false
    session.allowWriteRequests = false
    session.dirty = true
    session.version++
    session.frames.clear()
    await collectSecretState(session)
    return { ok: true }
  }
  if (command.operation === 'take') { session.dirty = true; session.frames.clear(); session.networkFrozen = false; session.allowWriteRequests = true }
  if (!['frame', 'take'].includes(command.operation)) {
    const frame = session.frames.get(command.frame_id)
    const live = await captureFrameState(page)
    if (!frame || frame !== live) return errorResult('stale_frame', 'page changed; refresh the frame')
    session.frames.clear() // input is one-use, including failed/uncertain input
    session.networkFrozen = false
    session.allowWriteRequests = true
    // Owner input may be a password, OTP or token even in an untyped field.
    if (command.text) session.secrets.add(command.text)
    try {
      switch (command.operation) {
        case 'click': await page.mouse.click(command.x, command.y, { button: command.button || 'left' }); break
        case 'double_click': await page.mouse.dblclick(command.x, command.y, { button: command.button || 'left' }); break
        case 'wheel': await page.mouse.move(command.x, command.y); await page.mouse.wheel(command.delta_x || 0, command.delta_y || 0); break
        case 'key': await page.keyboard.press(command.key); break
        case 'text': await page.keyboard.insertText(command.text || ''); break
        case 'navigate': await page.goto(command.url, { waitUntil: 'domcontentloaded', timeout: 12000 }); break
        default: return errorResult('invalid_control', 'unknown owner input')
      }
      session.dirty = true
      await collectSecretState(session)
      for (const frame of page.frames()) {
      const entered = await frame.evaluate(() => {
        const element = document.activeElement
        return element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement ? element.value : element?.isContentEditable ? element.innerText : ''
      }).catch(() => '')
      if (entered && entered.length >= 3) session.secrets.add(entered)
      }
    } finally { session.dirty = true }
  }
  // Owner-only pixels. No URL, DOM, inputs or browser state is published to
  // the model. Capture guards before and after the screenshot to reject drift.
  const capture = () => captureFrameState(page)
  const before = await capture()
  const jpeg = await page.screenshot({ type: 'jpeg', quality: 60, caret: 'initial', timeout: 5000 })
  const after = await capture()
  const id = crypto.randomUUID()
  if (before === after) session.frames.set(id, after)
  while (session.frames.size > 4) session.frames.delete(session.frames.keys().next().value)
  if (jpeg.length > 1024 * 1024) return errorResult('frame_limit', 'browser frame exceeded limit')
  return { ok: true, owner_frame: { jpeg: jpeg.toString('base64'), frame_id: id, width: 1280, height: 720, url: page.url() } }
}

// Coordinates can hit cross-origin child frames. Guard every document in the
// screenshot, including child navigation, scrolling and iframe replacement.
const frameIdentities = new WeakMap()
async function captureFrameState(page) {
  const frames = page.frames()
  const state = await Promise.all(frames.map(async frame => {
    if (!frameIdentities.has(frame)) frameIdentities.set(frame, crypto.randomUUID())
    const document = await frame.evaluate(() => ({ revision: window.__butlerRevision, documentID: window.__butlerDocumentID }))
    return { frame: frameIdentities.get(frame), ...document }
  }))
  if (frames.length !== page.frames().length || frames.some((frame, i) => frame !== page.frames()[i])) throw new Error('frame changed during capture')
  return JSON.stringify(state)
}

const lines = createInterface({ input: process.stdin, crlfDelay: Infinity })
try {
  for await (const line of lines) {
    let result
    try { result = await execute(JSON.parse(line)) }
    catch { result = errorResult('operation_failed', 'browser execution failed; observe before another action') }
    process.stdout.write(JSON.stringify(result) + '\n')
  }
} finally {
  for (const id of sessions.keys()) await closeSession(id)
  await browser?.close().catch(() => {})
}
