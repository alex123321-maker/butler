import { chromium, request as playwrightRequest } from 'playwright'
import { createInterface } from 'node:readline'
import { randomUUID } from 'node:crypto'

// Private JSON-lines protocol. Never print page diagnostics, request bodies,
// cookies, storage state, passwords or Playwright exceptions.
const sessions = new Map()
let browser

const errorResult = (error_type, message) => ({ ok: false, error: { error_type, message, retryable: false } })
const rejectedRead = reason => ({ ...errorResult(reason, 'GET navigation precondition failed'), read_rejection: 'pre_navigation_v1', navigation_dispatched: false })
const rejectedPreparation = reason => ({ ...errorResult(reason, 'Local preparation precondition failed'), preparation_rejection: 'pre_input_v1', input_dispatched: false })
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
  const root = session.root || session
  const pages = root.managed ? [...root.pages.values()].filter(view => !view.page.isClosed()).map(view => view.page) : [session.page]
  for (const page of pages) for (const frame of page.frames()) {
  const values = await frame.evaluate(({ managed }) => {
    const values = []
    for (const input of document.querySelectorAll(managed ? 'input[type=password],input[type=hidden]' : 'input[type=password]')) if (input.value) values.push(input.value)
    try { for (let i = 0; i < localStorage.length; i++) values.push(localStorage.getItem(localStorage.key(i))) } catch {}
    try { for (let i = 0; i < sessionStorage.length; i++) values.push(sessionStorage.getItem(sessionStorage.key(i))) } catch {}
    return values
  }, { managed: root.managed }).catch(() => [])
  for (const value of values) if (value) session.secrets.add(value)
  }
}

function rememberPageURL(session, raw) {
  if (!session.managed) return
  try { const url = new URL(raw); for (const value of url.searchParams.values()) if (value) session.secrets.add(value); if (url.hash) session.secrets.add(url.hash.slice(1)) } catch {}
}
function visibleURL(session, raw) {
  if (!(session.root || session).managed) return redact(session, raw)
  try { const url = new URL(raw); url.search = ''; url.hash = ''; return redact(session, url.href) } catch { return '' }
}
function attachPage(root, page, openerID = '') {
  const view = root.page ? Object.create(root) : root
  if (view !== root) {
    view.root = root
    for (const key of ['networkFrozen', 'allowWriteRequests', 'preparedDraft', 'loginGate']) Object.defineProperty(view, key, { get: () => root[key], set: value => { root[key] = value } })
  }
  view.page = page; view.pageID = randomUUID().replaceAll('-', ''); view.openerID = openerID
  view.documentID = randomUUID().replaceAll('-', ''); view.version = 0; view.dirty = true; view.state = 'pending'
  view.observedPageDocumentID = ''; view.observedPageURL = ''; view.observedDocumentID = undefined; view.observedRevision = undefined
  root.pages.set(view.pageID, view); root.pageIndex.set(page, view)
  page.on('framenavigated', frame => {
    view.dirty = true
    if (frame === page.mainFrame()) {
      view.documentID = randomUUID().replaceAll('-', ''); rememberPageURL(root, frame.url())
      if (root.managed && frame.url() !== 'about:blank' && origin(frame.url()) !== root.initialOrigin && !(root.ssoGate?.child === view.pageID && ssoURLAllowed(root.ssoGate, frame.url()))) {
        if (root.ssoGate) root.ssoFailure = 'sso_page_origin_rejected'
        void page.close().catch(() => {})
      }
    }
  })
  page.on('close', () => { view.state = 'closed'; view.dirty = true; root.frames.clear() })
  return view
}
function newPopupGate(root, view, duration) {
  return { opener: view.pageID, document: view.documentID, until: Date.now() + duration,
    remaining: Math.max(0, Math.min(4 - [...root.pages.values()].filter(page => !page.page.isClosed()).length, 32 - root.pages.size)) }
}
async function admitPopup(root, page, reservation) {
  if (root.pageIndex.has(page)) return root.pageIndex.get(page)
  if (root.ssoAttempt?.child && root.pages.get(root.ssoAttempt.child)?.page !== page) {
    root.ssoAttempt.violated = true; void page.close().catch(() => {}); return undefined
  }
  const gate = reservation || root.popupGate
  const parent = await page.opener().catch(() => null)
  const opener = root.pageIndex.get(parent)
  if (!root.managed || !gate || Date.now() > gate.until || !opener || opener.pageID !== gate.opener || opener.documentID !== gate.document || opener.page.isClosed() || root.pages.size >= 32 || [...root.pages.values()].filter(view => !view.page.isClosed()).length >= 4) {
    void page.close().catch(() => {}); return undefined
  }
  // The event and first-request route may race through opener(). Recheck the
  // map and cap after the await, before minting exactly one identity.
  if (root.pageIndex.has(page)) return root.pageIndex.get(page)
  if (root.pages.size >= 32 || [...root.pages.values()].filter(view => !view.page.isClosed()).length >= 4) { void page.close().catch(() => {}); return undefined }
  if (root.ssoGate?.child) { root.ssoGate.violated = true; void page.close().catch(() => {}); return undefined }
  const view = attachPage(root, page, opener.pageID)
  if (root.ssoGate) root.ssoGate.child = view.pageID
  if (reservation) view.initialRequestAdmitted = true
  return view
}
function reserveInitialPopupRequest(root, request, url) {
  // Chromium may issue the initial popup navigation before Playwright exposes
  // Request.frame(). Reserve a finite GET under the exact source gesture BEFORE
  // continuing it. Bind by this Request's eventual Frame/Page, never URL/title.
  const previous = root.initialRequests.get(request.redirectedFrom())
  const gate = previous || root.popupGate
  const opener = gate && root.pages.get(gate.opener)
  const originAllowed = root.ssoGate ? ssoURLAllowed(root.ssoGate, request.url()) : url.origin === root.initialOrigin
  if (!root.managed || !gate || Date.now() > gate.until || !opener || opener.documentID !== gate.document || opener.page.isClosed() || !request.isNavigationRequest() || request.method() !== 'GET' || !originAllowed) return false
  if (previous) { if (previous.hops >= 5) return false }
  else { if (gate.remaining <= 0) { if (root.ssoAttempt) root.ssoAttempt.violated = true; return false }; gate.remaining-- }
  const reservation = { opener: gate.opener, document: gate.document, until: Math.min(gate.until, Date.now() + 2000), hops: (previous?.hops || 0) + 1 }
  root.initialRequests.set(request, reservation)
  void request.response().then(async () => {
    try {
      const page = request.frame().page()
      const view = root.pageIndex.get(page) || await admitPopup(root, page, reservation)
      if (view) view.initialRequestAdmitted = true
    } catch { /* Missing identity grants no subsequent input or credentials. */ }
  }).catch(() => {})
  return true
}

async function newSession(request) {
  const browser = await ensureBrowser()
  if (!request.proxy_url) throw new Error('private egress proxy missing')
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 }, serviceWorkers: 'block', acceptDownloads: false, proxy: { server: request.proxy_url, bypass: '' } })
  const session = { context, proxyURL: request.proxy_url, page: undefined, scope: request.scope_origin || origin(request.url),
    version: 0, dirty: true, secrets: new Set(), frames: new Map(), allowedDomains: request.allowed_domains || [], expires: Date.parse(request.expires_at), expiryTimer: undefined, allowWriteRequests: false, networkFrozen: false,
    managed: request.managed_pages === true, initialOrigin: origin(request.url), pages: new Map(), pageIndex: new WeakMap(), initialRequests: new WeakMap() }
  // Native same-origin and cookie rules remain enabled. The private proxy
  // checks public DNS on every connection; this route retains contract caps.
  await context.route('**/*', async route => {
    const request = route.request()
    const url = new URL(request.url())
    const allowed = ['http:', 'https:'].includes(url.protocol) && !url.username && !url.password && (!session.allowedDomains.length || session.allowedDomains.some(domain => domain === '*' || url.hostname === domain || url.hostname.endsWith('.' + domain)))
    if (session.ssoGate) { await routeSSO(session, route, allowed); return }
    let popupRead = false
    if (session.managed) {
      rememberPageURL(session, request.url())
      let page
      try { page = request.frame().page() } catch {
        if (allowed && reserveInitialPopupRequest(session, request, url)) await route.continue()
        else await route.abort('blockedbyclient')
        return
      }
      const view = session.pageIndex.get(page) || await admitPopup(session, page)
      if (!view || request.isNavigationRequest() && request.frame() === page.mainFrame() && url.origin !== session.initialOrigin || !['GET','HEAD'].includes(request.method()) && (session.writePage !== page || url.origin !== session.initialOrigin)) {
        await route.abort('blockedbyclient')
        if (view && view !== session && request.isNavigationRequest() && url.origin !== session.initialOrigin) void page.close().catch(() => {})
        return
      }
      if (view !== session && !view.initialRequestAdmitted && request.isNavigationRequest() && request.frame() === page.mainFrame()) {
        const prior = session.initialRequests.get(request.redirectedFrom())
        const gate = prior || session.popupGate
        if (!gate || Date.now() > gate.until || gate.opener !== view.openerID || !prior && gate.remaining <= 0) { await route.abort('blockedbyclient'); return }
        if (!prior) gate.remaining--
        view.initialRequestAdmitted = true
      }
      popupRead = view !== session && session.popupGate && Date.now() <= session.popupGate.until && view.openerID === session.popupGate.opener && ['GET','HEAD'].includes(request.method()) && url.origin === session.initialOrigin
    }
    if (session.loginGate) {
      const gate = session.loginGate
      let pass = false
      if (!session.networkFrozen && allowed && url.origin === gate.origin && request.isNavigationRequest() && request.frame() === session.page.mainFrame()) {
        if (request.method() === 'POST' && gate.posts === 0 && request.url() === gate.target && !request.redirectedFrom() && request.headers()['content-type']?.split(';')[0] === 'application/x-www-form-urlencoded' && request.postData() === gate.body) {
          gate.posts++; gate.chain.add(request); pass = true
        } else if (request.method() === 'GET' && gate.posts === 1 && gate.hops < 5 && gate.chain.has(request.redirectedFrom())) {
          gate.hops++; gate.chain.add(request); pass = true
        }
      }
      if (pass) await route.continue(); else await route.abort('blockedbyclient')
      return
    }
    if (session.networkFrozen && !popupRead || !allowed || (!session.allowWriteRequests && !['GET', 'HEAD'].includes(route.request().method()))) await route.abort('blockedbyclient')
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
  context.on('page', page => {
    if (!session.page) attachPage(session, page)
    else if (session.managed) void admitPopup(session, page)
    else if (page !== session.page) void page.close().catch(() => {})
  })
  const page = await context.newPage()
  if (!session.pageIndex.has(page)) attachPage(session, page)
  return session
}

async function observe(session, selector = 'body') {
  const externalDocument = session.documentID
  await collectSecretState(session)
  // Read visible text and bounded form structure, never input values, cookies,
  // storage, screenshot pixels or full DOM into a normal tool artifact.
  // Capture text, URL and DOM revision in one JavaScript turn. A mutation
  // between separate reads must not validate an older text snapshot.
  const captured = await session.page.evaluate(({ selector, publicMetadata, frozen }) => {
    const element = document.querySelector(selector)
    if (!element) throw new Error('requested selector is not present')
    const links = selector === 'body' ? Array.from(document.querySelectorAll('a[href]')).slice(0, 30)
      .map(a => ({ text: (a.innerText || '').slice(0, 120), href: a.href })).filter(a => a.text && /^https?:/.test(a.href)) : []
    // Positional targets plus already-visible labels, never input values,
    // attributes, handlers or credential-derived validity/disabled state.
    const buttons = selector === 'body' ? [...document.querySelectorAll('button')].slice(0, 64)
      .map((button, index) => ({ selector: `button >> nth=${index}`, text: (button.innerText || '').trim().slice(0, 120) }))
      .filter(button => button.text) : []
    const ordinaryTypes = new Set(['text', 'search', 'number', 'email', 'url', 'tel', 'textarea', 'select-one', 'select-multiple', 'checkbox', 'radio', 'submit', 'button', 'reset'])
    const issueNames = { valueMissing: 'value_missing', typeMismatch: 'type_mismatch', rangeUnderflow: 'range_underflow', rangeOverflow: 'range_overflow', stepMismatch: 'step_mismatch', tooShort: 'too_short', tooLong: 'too_long', badInput: 'bad_input', patternMismatch: 'pattern_mismatch', customError: 'custom_error' }
    const forms = selector === 'body' ? Array.from(document.forms).slice(0, 10).map((form, index) => {
      const controls = Array.from(form.elements).slice(0, 129)
      // A password/hidden/file/unknown control suppresses metadata for the
      // entire form, including username fields. Never read a field's value.
      const ordinary = publicMetadata && controls.length <= 128 && controls.every(e => ordinaryTypes.has(String(e.type || '').toLowerCase()))
      return { selector: `form >> nth=${index}`, method: form.method.toUpperCase(), action: form.action.slice(0, 2048),
        fields: controls.filter(e => e.name).slice(0, 32).map(e => {
          const field = { name: e.name.slice(0, 128), type: String(e.type || e.tagName).slice(0, 32) }
          if (!ordinary || ['submit', 'button', 'reset'].includes(field.type)) return field
          field.constraints = { required: e.required === true }
          if (field.type === 'number') for (const key of ['min', 'max', 'step']) {
            const value = e.getAttribute(key)
            if (value && value.length <= 64) field.constraints[key] = value
          }
          for (const [property, key] of [['minLength', 'min_length'], ['maxLength', 'max_length']]) {
            if (Number.isInteger(e[property]) && e[property] >= 0 && e[property] <= 1000000) field.constraints[key] = e[property]
          }
          // Reading ValidityState does not invoke checkValidity/reportValidity
          // (which dispatch invalid events). Observe flags only with all routes
          // frozen; the flags prove no external submission or outcome.
          if (frozen && e.validity) field.validation = { valid: e.validity.valid === true,
            issues: Object.entries(issueNames).filter(([property]) => e.validity[property] === true).map(([, name]) => name) }
          return field
        }) }
    }) : []
    return { text: element.innerText || '', links, buttons, forms, title: document.title, url: location.href,
      revision: window.__butlerRevision, documentID: window.__butlerDocumentID }
  }, { selector, publicMetadata: session.secrets.size === 0, frozen: session.networkFrozen })
  await collectSecretState(session)
  const { text, links, title } = captured
  if (session.managed && externalDocument !== session.documentID) throw new Error('document changed during observation')
  const final_url = visibleURL(session, captured.url)
  session.observedPageDocumentID = externalDocument
  session.observedPageURL = final_url
  session.observedPrivateURL = captured.url
  session.state = 'ready'
  session.observedRevision = captured.revision
  session.observedDocumentID = captured.documentID
  session.version++
  session.dirty = false
  const cleanText = Array.from(redact(session, text)).slice(0, 12000).join('')
  const page_status = /captcha|verify you are human|access denied|checking your browser|403 forbidden/i.test(cleanText + title) ? 'blocked' : 'ok'
  return { ok: true, observation_version: session.version, final_url, title: redact(session, title), text: cleanText, page_status, network_state: session.networkFrozen ? 'frozen' : 'read_only',
    forms: captured.forms.map(form => ({ ...form, action: visibleURL(session, form.action), fields: form.fields.map(field => {
      const safe = { name: redact(session, field.name), type: field.type }
      // State may have gained a secret during capture. Suppress rather than
      // expose validity/length feedback or secret-derived constraint strings.
      if (!session.secrets.size) {
        if (field.constraints) safe.constraints = field.constraints
        if (field.validation) safe.validation = field.validation
      }
      return safe
    }) })),
    buttons: captured.buttons.map(button => ({ selector: button.selector, text: redact(session, button.text) })),
    links: links.map(link => ({ text: redact(session, link.text), href: visibleURL(session, link.href) })) }
}

let spaSession
async function closeSPA() {
  const current = spaSession
  spaSession = undefined
  if (!current) return
  clearTimeout(current.timer)
  await current.browser.close().catch(() => {})
  current.session.secrets.clear()
}

async function executeSPA(request) {
  const config = request.spa
  if (!config || !config.args || config.args.url !== request.url) return errorResult('spa_profile_denied', 'bounded SPA profile required')
  if (config.phase === 'observe') {
    const current = spaSession
    if (!current || config.nonce !== current.nonce || current.args !== JSON.stringify(config.args)) return errorResult('spa_session_lost', 'SPA observation context is unavailable')
    try {
      current.session.networkFrozen = true
      return await observe(current.session)
    } finally { await closeSPA() }
  }
  if (config.phase !== 'run' || spaSession || !/^[A-Za-z0-9+/]{43}=$/.test(config.spki || '') || !request.proxy_url || !Number.isFinite(Date.parse(request.expires_at)) || Date.parse(request.expires_at) <= Date.now()) return errorResult('spa_profile_denied', 'fresh bounded SPA context required')
  const relay = new URL(request.proxy_url)
  if (relay.protocol !== 'http:' || relay.hostname !== '127.0.0.1' || relay.username || relay.password || relay.pathname !== '/') return errorResult('spa_profile_denied', 'private SPA relay required')
  const privateBrowser = await chromium.launch({ headless: true, proxy: { server: request.proxy_url, bypass: '' }, args: ['--disable-quic', '--disable-background-networking', '--force-webrtc-ip-handling-policy=disable_non_proxied_udp', '--proxy-bypass-list=<-loopback>', '--ignore-certificate-errors-spki-list=' + config.spki] })
  let ready = false
  try {
    const context = await privateBrowser.newContext({ viewport: { width: 1280, height: 720 }, serviceWorkers: 'allow', acceptDownloads: false })
    // All HTTPS is terminated by the Go relay, including worker script updates
    // that Playwright page routes cannot intercept. Remote TLS stays verified.
    const page = await context.newPage()
    context.on('page', extra => { if (extra !== page) void extra.close().catch(() => {}) })
    const session = { context, page, scope: new URL(request.url).origin, version: 0, secrets: new Set(), frames: new Map(), managed: false, networkFrozen: false }
    const nonce = randomUUID()
    spaSession = { browser: privateBrowser, session, nonce, args: JSON.stringify(config.args), timer: setTimeout(() => { void closeSPA() }, Math.max(1, Date.parse(request.expires_at) - Date.now())) }
    await page.goto(request.url, { waitUntil: 'domcontentloaded', timeout: 8000 })
    if (config.args.ready_selector) await page.locator(config.args.ready_selector).waitFor({ state: 'visible', timeout: 4000 })
    const settle = config.args.settle_ms || 1000
    await page.waitForTimeout(settle)
    if (config.args.filter) {
      const selector = page.locator(config.args.filter.selector)
      if (await selector.count() !== 1) throw new Error('ambiguous public filter')
      await selector.evaluate((element, value) => {
        const type = String(element.type || '').toLowerCase()
        const rect = element.getBoundingClientRect()
        const style = getComputedStyle(element)
        if (!element.isConnected || element.disabled || element.readOnly || rect.width <= 0 || rect.height <= 0 || style.visibility !== 'visible' || style.display === 'none' || !['text', 'search', 'select-one'].includes(type)) throw new Error('unsupported public filter')
        if (type === 'select-one') {
          const matches = [...element.options].filter(option => option.value === value && !option.disabled)
          if (matches.length !== 1) throw new Error('unsupported filter option')
          element.value = value
        } else {
          Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(element, value)
        }
        element.dispatchEvent(new Event('input', { bubbles: true }))
        element.dispatchEvent(new Event('change', { bubbles: true }))
      }, config.args.filter.value)
      await page.waitForTimeout(settle)
    }
    if (new URL(page.url()).origin !== session.scope) throw new Error('SPA origin changed')
    ready = true
    return { ok: true, spa_nonce: nonce }
  } finally {
    if (!ready) {
      await closeSPA()
      await privateBrowser.close().catch(() => {})
    }
  }
}

async function executeSingle(request) {
  const tool = request.tool_name
  if (tool === 'browser.read_spa') return executeSPA(request)
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
  const root = sessions.get(request.browser_session_id)
  if (!root) return errorResult('session_lost', 'browser session was closed, expired, or lost; explicitly open a new session')
  if (Date.now() >= root.expires) { await closeSession(request.browser_session_id); return errorResult('session_expired', 'browser session expired') }
  if (tool === 'owner.control') return await ownerControl(root, request.owner_command)
  if (tool === 'browser.close') { await closeSession(request.browser_session_id); return { ok: true } }
  if (root.managed && tool === 'browser.pages') return { ok: true }
  const session = root.managed ? root.pages.get(request.page_id) : root
  if (!session || session.page.isClosed()) return errorResult('page_lost', 'exact page is unavailable; refresh pages')
  if (root.managed && (request.document_id !== session.documentID || request.document_id !== session.observedPageDocumentID || request.observation_version !== session.version || request.expected_url !== visibleURL(session, session.page.url()))) return errorResult('stale_page', 'page document changed; refresh pages')
  if (tool === 'browser.freeze_snapshot') {
    session.networkFrozen = true
    // A later public GET reader must not release this observed private state.
    session.preparedDraft = true
    return await observe(session)
  }
  const localPreparation = ['browser.prepare_fill', 'browser.prepare_type'].includes(tool)
  const getForm = tool === 'browser.submit_get_form' || tool === 'browser.follow_link'
  // Freeze before reading the live guards or injecting any new value. A stale
  // preparation can fail safely without resuming background page requests.
  const login = tool === 'browser.login'
  if (localPreparation || getForm || login) session.networkFrozen = true
  if (session.networkFrozen && ['browser.navigate', 'browser.fill', 'browser.type'].includes(tool) && !root.managed) return errorResult('network_frozen', 'prepared context permits local preparation, observation, close or separately authorized click only')
  session.scope = origin(session.page.url())
  if (tool === 'browser.snapshot') return await observe(session)
  if (tool === 'browser.extract_text') return await observe(session, request.selector)
  // Re-check live page state immediately before every effect. Go already
  // checks trusted ownership, exclusive control, and the last observed state.
  await collectSecretState(session)
  const live = await session.page.evaluate(() => ({ revision: window.__butlerRevision, documentID: window.__butlerDocumentID }))
  if (request.observation_version !== session.version || session.dirty || live.revision !== session.observedRevision || live.documentID !== session.observedDocumentID) return getForm ? rejectedRead('stale_observation') : localPreparation ? rejectedPreparation('stale_observation') : errorResult('stale_observation', 'page changed since observation; take a new snapshot')
  if (request.expected_url !== visibleURL(session, session.page.url())) return getForm ? rejectedRead('page_changed') : localPreparation ? rejectedPreparation('page_changed') : errorResult('page_changed', 'current page does not match expected_url; take a new snapshot')
  if (tool === 'browser.page_close') { await session.page.close(); return { ok: true } }
  if (getForm) return await submitGetForm(session, request, live)
  if (login) return await submitLogin(session, request, live)
  // An ordinary local draft is private too, even without credential_ref.
  // A later read tool must never thaw input callbacks or release that draft.
  if (['browser.fill','browser.type','browser.prepare_fill','browser.prepare_type'].includes(tool)) session.preparedDraft = true
  for (const secret of request.secret_values || []) if (secret) session.secrets.add(secret)
  const timeout = request.timeout || 5000
  let credentialVerified
  if (root.managed && ['browser.click','browser.fill','browser.type'].includes(tool)) {
    root.popupGate = newPopupGate(root, session, 6000)
    root.writePage = session.page
  }
  session.allowWriteRequests = ['browser.click', 'browser.fill', 'browser.type'].includes(tool)
  // The Go proxy opens a bounded click window, but page traffic remains frozen
  // until the exact live observation and URL guards above succeed.
  if (tool === 'browser.click' || root.managed && ['browser.fill','browser.type'].includes(tool)) session.networkFrozen = false
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
        const typedPrefix = request.secret_values?.length && (tool === 'browser.type' || tool === 'browser.prepare_type') ? await locator.inputValue({ timeout }) : ''
        if (tool === 'browser.fill' || tool === 'browser.prepare_fill') await locator.fill(request.value || '', { timeout })
        else await locator.pressSequentially(request.text || '', { timeout })
        if (request.secret_values?.length) {
          // Compare only the exact value privately delivered for this operation.
          // Never return a value, digest, length or per-character feedback.
          const expected = (tool === 'browser.fill' || tool === 'browser.prepare_fill') ? request.value : typedPrefix + request.text
          credentialVerified = await locator.evaluate((element, value) => 'value' in element && element.value === value, expected, { timeout })
          if (!credentialVerified) return errorResult('credential_mismatch', 'page did not retain the authorized credential value')
        }
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
    if (tool === 'browser.click' || root.managed && ['browser.fill','browser.type'].includes(tool)) session.networkFrozen = true
    return { ...await observe(session), ...(localPreparation ? { local_preparation: true } : {}), ...(credentialVerified === undefined ? {} : { credential_verified: credentialVerified }) }
  } catch (err) {
    session.version++
    session.dirty = true
    return { ...safeError(err), observation_version: session.version, final_url: redact(session, session.page.url()) }
  } finally {
    session.allowWriteRequests = false
    if (tool === 'browser.click' || localPreparation) session.networkFrozen = true
  }
}

// Two private phases. No click, submit, input, formdata or page handler runs.
// This supports public GET search/navigation, never authentication or POST.
async function submitGetForm(session, request, live) {
  const denied = () => rejectedRead('get_form_denied')
  try {
    if (session.secrets.size || session.preparedDraft) return denied()
    if (request.get_form_phase === 'plan') {
      session.getFormPlan = undefined
      const compiled = request.tool_name === 'browser.follow_link' ? await session.page.locator(request.selector).evaluate(link => {
        if (!(link instanceof HTMLAnchorElement) || !['','_self'].includes(link.target) || link.hasAttribute('download')) return null
        const target = new URL(link.href)
        if (target.origin !== location.origin || target.username || target.password || !['http:','https:'].includes(target.protocol) || target.href.length > 16384) return null
        return target.href
      }) : await session.page.locator(request.selector).evaluate((form, fields) => {
        if (!(form instanceof HTMLFormElement) || form.method.toLowerCase() !== 'get' || !['', '_self'].includes(form.target)) return null
        const action = new URL(form.action)
        if (action.origin !== location.origin || action.username || action.password || !['http:', 'https:'].includes(action.protocol)) return null
        const controls = Array.from(form.elements)
        if (controls.length > 32) return null
        const names = new Set(), entries = []
        for (const element of controls) {
          const type = String(element.type || '').toLowerCase()
          if (['password', 'file', 'hidden'].includes(type)) return null
          if (element.disabled || !element.name || ['submit', 'button', 'reset'].includes(type)) continue
          const name = element.name
          if (name.length > 128 || /password|passwd|pwd|token|secret|credential|cookie|session|csrf|xsrf|otp|api.?key|auth/i.test(name)) return null
          if (!['text', 'search', 'number', 'email', 'tel', 'url', 'checkbox', 'radio', 'textarea', 'select-one', 'select-multiple', 'range', 'date'].includes(type)) return null
          names.add(name)
          if (Object.hasOwn(fields, name)) { entries.push([name, fields[name]]); continue }
          if (['checkbox', 'radio'].includes(type) && !element.checked) continue
          if (type.startsWith('select-')) {
            for (const option of element.selectedOptions) entries.push([name, option.value])
          } else entries.push([name, element.value])
        }
        if (Object.keys(fields).some(name => !names.has(name)) || entries.some(([name, value]) => typeof value !== 'string' || value.length > 4096)) return null
        action.search = new URLSearchParams(entries).toString()
        action.hash = ''
        return action.href.length <= 16384 ? action.href : null
      }, request.fields)
      if (!compiled) return denied()
      const nonce = randomUUID()
      session.getFormPlan = { target: compiled, nonce, version: session.version, documentID: live.documentID, revision: live.revision }
      return { ok: true, get_form_target: compiled, get_form_nonce: nonce }
    }
    const plan = session.getFormPlan
    session.getFormPlan = undefined // one-use even if navigation fails
    if (request.get_form_phase !== 'navigate' || !plan || plan.nonce !== request.get_form_nonce || plan.version !== request.observation_version || plan.documentID !== live.documentID || plan.revision !== live.revision) return denied()
    session.allowWriteRequests = false
    session.networkFrozen = false
    await session.page.goto(plan.target, { waitUntil: 'domcontentloaded', timeout: request.timeout || 15000 })
    session.networkFrozen = true
    return await observe(session)
  } catch (err) {
    session.version++
    session.dirty = true
    return { ...safeError(err), observation_version: session.version, final_url: redact(session, session.page.url()) }
  } finally {
    session.allowWriteRequests = false
    session.networkFrozen = true
  }
}

// Scoped login is distinct from click. The gate inspects the actual native
// POST (including formdata-handler changes), and admits only its GET redirects.
// Values, hidden fields, cookies and nonce stay on the private helper pipe.
async function submitLogin(session, request, live) {
  const denied = () => errorResult('login_denied', 'login precondition failed; no automatic retry')
  const scope = request.login?.scope
  if (!scope || scope.expected_url !== request.expected_url || scope.browser_session_id !== request.browser_session_id || origin(scope.target_url) !== origin(request.expected_url)) return denied()
  for (const secret of request.secret_values || []) if (secret) session.secrets.add(secret)
  session.preparedDraft = true
  try {
    if (request.login_phase === 'plan') {
      session.loginPlan = undefined
      // No page handler runs while compiling the permitted body.
      const entries = await session.page.evaluate(({ scope, username, password }) => {
        const select = selector => {
          const indexed = /^(.*) >> nth=(0|[1-9][0-9]{0,2})$/.exec(selector)
          const css = indexed ? indexed[1] : selector
          if (css.includes('>>')) return null
          const matches = document.querySelectorAll(css)
          if (!indexed && matches.length !== 1) return null
          return matches[indexed ? Number(indexed[2]) : 0] || null
        }
        const form = select(scope.form_selector)
        const user = select(scope.username_selector)
        const pass = select(scope.password_selector)
        if (!(form instanceof HTMLFormElement) || form.method.toLowerCase() !== 'post' || form.enctype !== 'application/x-www-form-urlencoded' || !['', '_self'].includes(form.target) || form.action !== scope.target_url) return null
        if (!(user instanceof HTMLInputElement) || !(pass instanceof HTMLInputElement) || user.form !== form || pass.form !== form || !['text','email'].includes(user.type) || pass.type !== 'password' || !user.name || !pass.name || user.disabled || pass.disabled || user === pass) return null
        const controls = Array.from(form.elements)
        if (controls.length > 32 || controls.filter(e => e.type === 'password').length !== 1) return null
        const entries = [], names = new Set()
        for (const e of controls) {
          if (e.disabled || !e.name || ['submit','button','reset'].includes(e.type)) continue
          if (!(e instanceof HTMLInputElement) || (e !== user && e !== pass && e.type !== 'hidden') || names.has(e.name) || e.name.length > 128) return null
          if (e !== user && e !== pass && !/^(?:_?csrf(?:_token)?|csrfmiddlewaretoken|_?xsrf(?:_token)?|authenticity_token|__requestverificationtoken)$/i.test(e.name)) return null
          const value = e === user ? username : e === pass ? password : e.value
          if (typeof value !== 'string' || value.length > 4096) return null
          names.add(e.name); entries.push([e.name, value])
        }
        return entries
      }, request.login)
      if (!entries || entries.length < 2) return denied()
      for (const [, value] of entries) if (value) session.secrets.add(value)
      const nonce = randomUUID()
      session.loginPlan = { nonce, entries, version: session.version, documentID: live.documentID, revision: live.revision, scope: JSON.stringify(scope) }
      return { ok: true, login_nonce: nonce }
    }
    const plan = session.loginPlan
    session.loginPlan = undefined
    if (request.login_phase !== 'submit' || !plan || plan.nonce !== request.login_nonce || plan.version !== request.observation_version || plan.documentID !== live.documentID || plan.revision !== live.revision || plan.scope !== JSON.stringify(scope)) return denied()
    const gate = { origin: origin(scope.target_url), target: scope.target_url, body: new URLSearchParams(plan.entries).toString(), posts: 0, hops: 0, chain: new Set() }
    session.loginGate = gate
    // Fill and invoke native submission in one document turn. Background input,
    // XHR, fetch, iframe, popup, websocket and additional POST traffic stay blocked.
    session.networkFrozen = false
    await Promise.all([
      session.page.waitForURL(url => url.href !== scope.expected_url, { waitUntil: 'domcontentloaded', timeout: 12000 }),
      session.page.evaluate(({ scope, username, password }) => {
        const select = selector => {
          const indexed = /^(.*) >> nth=(0|[1-9][0-9]{0,2})$/.exec(selector)
          const css = indexed ? indexed[1] : selector
          if (css.includes('>>')) return null
          const matches = document.querySelectorAll(css)
          if (!indexed && matches.length !== 1) return null
          return matches[indexed ? Number(indexed[2]) : 0] || null
        }
        const form = select(scope.form_selector)
        const user = select(scope.username_selector)
        const pass = select(scope.password_selector)
        if (!(form instanceof HTMLFormElement) || user?.form !== form || pass?.form !== form || form.action !== scope.target_url) throw new Error('login changed')
        const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set
        set.call(user, username); set.call(pass, password)
        HTMLFormElement.prototype.submit.call(form)
      }, request.login)
    ])
    session.networkFrozen = true
    const observed = await observe(session)
    return { ...observed, login_posts: gate.posts, login_url: session.page.url(), login_cookies: await session.context.cookies() }
  } catch {
    session.version++; session.dirty = true
    return errorResult('login_unknown', 'login was not verified; observe before any further action and never repeat the POST automatically')
  } finally {
    session.loginGate = undefined
    session.networkFrozen = true
    session.allowWriteRequests = false
  }
}

function ssoWithin(scope, raw) {
  try {
    const u = new URL(raw), p = u.pathname || '/'
    if (u.protocol !== 'https:' || u.username || u.password || u.origin.toLowerCase() !== scope.origin.toLowerCase() || /%2f|%5c|%2e|\\/i.test(p)) return false
    const prefix = scope.path_prefix.replace(/\/$/, '')
    return !prefix || p === prefix || p.startsWith(prefix + '/')
  } catch { return false }
}
function ssoURLAllowed(gate, raw) {
  if (!gate || Date.now() >= Date.parse(gate.dispatch.expires_at)) return false
  const site = origin(raw)
  return [origin(gate.scope.rp_url), gate.scope.idp_origin || origin(gate.scope.idp_url)].includes(site) &&
    !(gate.dispatch.denied || []).some(s => ssoWithin(s, raw)) && gate.dispatch.allowed.some(s => ssoWithin(s, raw))
}

// The private dispatch contains Broker-derived path caps. A consented origin is
// never permission for an unrelated path, page, background request or POST.
async function routeSSO(root, route, operatorAllowed) {
  const request = route.request(), gate = root.ssoGate
  const reject = (reason = 'sso_route_rejected') => { root.ssoFailure = reason; return route.abort('blockedbyclient') }
  const proceed = () => continueSSONavigation(root, route, gate)
  if (!operatorAllowed || root.networkFrozen || !ssoURLAllowed(gate, request.url()) || !request.isNavigationRequest()) return reject('sso_route_cap')
  if (gate.inspection) {
    // Apply before frame lookup: the first popup redirect chain can precede
    // the page event, including a cookie-backed callback with no IdP document.
    if (origin(request.url()) === gate.scope.idp_origin) gate.seenProvider = true
    else if (gate.seenProvider) return reject()
  }
  let page, frame
  try { frame = request.frame(); page = frame.page() } catch {
    if (gate.phase === 'plan' && reserveInitialPopupRequest(root, request, new URL(request.url()))) {
      if (!gate.bootstrapRequest) gate.bootstrapRequest = request
      return proceed()
    }
    return reject('sso_route_identity')
  }
  const view = root.pageIndex.get(page) || (gate.phase === 'plan' ? await admitPopup(root, page) : undefined)
  if (!view || frame !== page.mainFrame()) return reject('sso_route_identity')
  const read = ['GET', 'HEAD'].includes(request.method())
  if (view.pageID === gate.source) {
    if (gate.inspection && request.url() !== gate.scope.rp_url) return reject()
    const allowed = gate.phase === 'plan' ? origin(request.url()) === origin(gate.scope.rp_url) : request.url() === gate.scope.verify_url && gate.posts === 1
    if (!read || !allowed || gate.rootReads++ >= 6) return reject()
    return proceed()
  }
  if (view.pageID !== gate.child || view.openerID !== gate.source) return reject('sso_route_child')
  if (gate.phase === 'plan') {
    if (!read || gate.hops++ >= 8) return reject()
    if (gate.inspection) {
      // Discovery must not complete cookie-backed SSO before owner consent.
      // RP bootstrap GETs may precede IdP, but no return/callback is admitted.
      if (origin(request.url()) === gate.scope.idp_origin) gate.seenProvider = true
      else if (gate.seenProvider) return reject()
    }
    if (!view.initialRequestAdmitted) {
      let reservation = root.initialRequests.get(request.redirectedFrom())
      // Our redirect bridge is a fresh navigation, not redirectedFrom(). Bind
      // the already-spent first-request reservation to its actual Page before
      // accepting it; another popup may not borrow that reservation.
      if (!reservation && gate.bootstrapRequest) {
        try { if (gate.bootstrapRequest.frame().page() === page) reservation = root.initialRequests.get(gate.bootstrapRequest) } catch {}
      }
      const popup = reservation || root.popupGate
      if (!popup || Date.now() > popup.until || popup.opener !== gate.source || !reservation && popup.remaining <= 0) return reject('sso_route_window')
      if (!reservation) popup.remaining--
      view.initialRequestAdmitted = true
    }
    return proceed()
  }
  if (gate.phase !== 'submit') return reject()
  if (request.method() === 'POST' && gate.posts === 0 && !request.redirectedFrom() && request.url() === gate.scope.target_url && request.headers()['content-type']?.split(';')[0] === 'application/x-www-form-urlencoded' && request.postData() === gate.body) {
    gate.posts++; return proceed()
  }
  if (request.method() === 'GET' && gate.posts === 1 && gate.hops++ < 5 && gate.redirectNext === request.url()) { gate.redirectNext = ''; return proceed() }
  return reject()
}

// Playwright routes do not intercept every redirect in a browser navigation.
// SSO therefore fetches exactly one authorized response through the same
// context's TLS-verified proxy and browser-selected cookies, without retries
// or automatic redirects.
// An allowed redirect becomes a fresh browser navigation, so the next URL is
// checked BEFORE it reaches the recipient. Never fulfill a provider body under
// the RP origin, and never follow a provider-to-RP redirect before consent.
async function continueSSONavigation(root, route, gate) {
  let response, transport
  try {
    const timeout = Math.min(6000, Date.parse(gate.dispatch.expires_at) - Date.now())
    if (timeout <= 0 || root.networkFrozen || root.ssoGate !== gate) return route.abort('blockedbyclient')
    // Preserve Chromium's actual cookie selection, including absence. A fresh
    // empty one-response jar cannot fill a header withheld by SameSite/CHIPS.
    const headers = await route.request().allHeaders()
    // Never share APIRequestContext's cookie store with Chromium: its response
    // parsing/addCookies path is not the browser's Set-Cookie policy. Only the
    // browser processes response cookies when the route is fulfilled below.
    transport = await playwrightRequest.newContext({ proxy: { server: root.proxyURL, bypass: '' }, ignoreHTTPSErrors: false, storageState: { cookies: [], origins: [] } })
    response = await transport.fetch(route.request(), { headers, maxRedirects: 0, maxRetries: 0, timeout })
    for (const cookie of (await transport.storageState()).cookies) if (cookie.value) root.secrets.add(cookie.value)
    if (root.networkFrozen || root.ssoGate !== gate || Date.now() >= Date.parse(gate.dispatch.expires_at)) return route.abort('blockedbyclient')
    if (response.status() >= 300 && response.status() < 400) {
      const location = response.headers().location
      if (!location) return route.abort('blockedbyclient')
      const next = new URL(location, route.request().url())
      const provider = gate.scope.idp_origin || origin(gate.scope.idp_url)
      if (!ssoURLAllowed(gate, next.href) || gate.phase === 'plan' && (gate.seenProvider || origin(route.request().url()) === provider) && next.origin !== provider || !['GET', 'HEAD'].includes(route.request().method()) && ![301, 302, 303].includes(response.status())) {
        gate.violated = true; return route.abort('blockedbyclient')
      }
      if (gate.phase === 'submit') gate.redirectNext = next.href
      const target = next.href.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
      const redirectHeaders = { 'content-type': 'text/html; charset=utf-8', 'cache-control': 'no-store', 'referrer-policy': 'no-referrer' }
      const setCookie = response.headers()['set-cookie']
      if (setCookie) redirectHeaders['set-cookie'] = setCookie
      return await route.fulfill({ status: 200, headers: redirectHeaders,
        body: `<!doctype html><meta http-equiv="refresh" content="0;url=${target}">` })
    }
    return await route.fulfill({ response })
  } catch (error) {
    const message = String(error?.message || '')
    root.ssoFailure = /certificate|SSL|TLS/i.test(message) ? 'sso_tls_rejected' : /proxy|ECONN|ENOTFOUND/i.test(message) ? 'sso_connection_rejected' : /timeout/i.test(message) ? 'sso_network_timeout' : 'sso_navigation_rejected'
    return route.abort('blockedbyclient').catch(() => {})
  }
  finally { await response?.dispose().catch(() => {}); await transport?.dispose().catch(() => {}) }
}

async function inspectSSO(root, request) {
  let stage = 'source'
  const denied = () => errorResult(root?.ssoFailure || `sso_inspection_${stage}`, 'SSO form inspection unavailable; no form was submitted')
  if (root) root.ssoFailure = undefined
  const scope = request.sso_inspection?.scope, dispatch = request.sso_inspection?.dispatch
  const source = root?.pages.get(request.page_id)
  if (!root?.managed || !scope || !dispatch || !source || root.ssoPlan || source.page.isClosed() ||
    source.documentID !== request.document_id || source.version !== request.observation_version || source.observedPageURL !== scope.rp_url ||
    scope.browser_session_id !== request.browser_session_id || scope.rp_page.page_id !== source.pageID || scope.rp_page.document_id !== source.documentID) return denied()
  root.networkFrozen = true
  const gate = { scope, dispatch, source: source.pageID, child: '', phase: 'plan', posts: 0, rootReads: 0, hops: 0, inspection: true, seenProvider: false }
  try {
    const live = await source.page.evaluate(() => ({ documentID: window.__butlerDocumentID, revision: window.__butlerRevision }))
    if (source.dirty || source.observedDocumentID !== live.documentID || source.observedRevision !== live.revision || source.observedPrivateURL !== source.page.url()) return denied()
    root.ssoAttempt = gate; root.ssoGate = gate
    stage = 'popup'
    root.popupGate = newPopupGate(root, source, 2000); root.popupGate.remaining = Math.min(1, root.popupGate.remaining)
    if (root.popupGate.remaining < 1) return denied()
    const popup = source.page.waitForEvent('popup', { timeout: 6000 }).catch(() => null)
    root.networkFrozen = false
    await source.page.locator(scope.trigger_selector).click({ timeout: 2000, noWaitAfter: true })
    const page = await popup
    if (!page) return denied()
    const child = root.pageIndex.get(page) || await admitPopup(root, page)
    if (!child || gate.child !== child.pageID || child.openerID !== source.pageID) return denied()
    stage = 'provider'
    await page.waitForURL(u => u.origin === scope.idp_origin, { waitUntil: 'domcontentloaded', timeout: 6000 })
    stage = 'provider_check'
    root.networkFrozen = true; root.popupGate = undefined
    if (gate.violated) { stage = 'popup_limit'; return denied() }
    if (source.documentID !== request.document_id || source.page.url() !== scope.rp_url) { stage = 'source_changed'; return denied() }
    if (page.isClosed() || !ssoURLAllowed(gate, page.url())) return denied()
    await collectSecretState(root)
    stage = 'form'
    const forms = await page.evaluate(expectedOrigin => {
      const url = new URL(location.href)
      if (url.origin !== expectedOrigin) return null
      const allForms = [...document.forms], inputs = [...document.querySelectorAll('input')]
      if (allForms.length > 8 || inputs.length > 128) return null
      const candidates = []
      for (const [index, form] of allForms.entries()) {
        if (form.method.toLowerCase() !== 'post' || form.enctype.toLowerCase() !== 'application/x-www-form-urlencoded') continue
        const target = new URL(form.action)
        if (target.origin !== expectedOrigin || target.search || target.hash || target.username || target.password) continue
        const owned = inputs.filter(input => input.form === form && !input.disabled && !input.readOnly)
        const users = owned.filter(input => ['text', 'email'].includes(input.type) && input.name)
        const passwords = owned.filter(input => input.type === 'password' && input.name)
        if (users.length !== 1 || passwords.length !== 1) continue
        candidates.push({ idp_url: url.origin + url.pathname, target_url: target.href,
          form_selector: `form >> nth=${index}`, username_selector: `input >> nth=${inputs.indexOf(users[0])}`, password_selector: `input >> nth=${inputs.indexOf(passwords[0])}` })
      }
      return candidates.length > 0 && candidates.length <= 4 ? candidates : null
    }, scope.idp_origin)
    if (!forms || gate.violated || forms.some(form => Object.values(form).some(value => redact(root, value) !== value || redact(child, value) !== value))) return denied()
    stage = 'cleanup'
    // Capture identity privately, never the IdP body's text or field values.
    const childLive = await page.evaluate(() => ({ documentID: window.__butlerDocumentID, revision: window.__butlerRevision }))
    child.observedPageDocumentID = child.documentID
    child.observedPageURL = visibleURL(child, page.url()); child.version++
    child.observedDocumentID = childLive.documentID; child.observedRevision = childLive.revision
    await page.close()
    if (gate.violated || source.documentID !== request.document_id || source.page.url() !== scope.rp_url) return denied()
    return { ...await observe(source), inspection_forms: forms }
  } catch (error) {
    const message = String(error?.message || '')
    if (root.ssoFailure) { /* Preserve the fixed private guard reason. */ }
    else if (/closed|detached/i.test(message)) root.ssoFailure = 'sso_page_lost'
    else if (/ERR_BLOCKED_BY_CLIENT/i.test(message)) root.ssoFailure = 'sso_route_rejected'
    else if (/ERR_FAILED/i.test(message)) root.ssoFailure = 'sso_request_failed'
    else if (/ERR_ABORTED|interrupted/i.test(message)) root.ssoFailure = 'sso_navigation_changed'
    else if (/timeout/i.test(message)) root.ssoFailure = 'sso_network_timeout'
    else if (error instanceof TypeError) root.ssoFailure = 'sso_type_rejected'
    else root.ssoFailure = 'sso_observation_rejected'
    return denied()
  }
  finally {
    root.networkFrozen = true; root.allowWriteRequests = false; root.popupGate = undefined
    const child = root.pages.get(gate.child)
    if (child && !child.page.isClosed()) await child.page.close().catch(() => {})
    root.ssoGate = undefined; root.ssoAttempt = undefined
  }
}

async function executeSSO(root, request) {
  const denied = () => errorResult(root?.ssoFailure || 'sso_unknown', 'SSO was not verified; never repeat the POST automatically')
  if (root) root.ssoFailure = undefined
  const scope = request.sso?.scope, dispatch = request.sso?.dispatch
  const source = root?.pages.get(request.page_id)
  if (!root?.managed || !scope || !dispatch || !source || source.page.isClosed() || source.documentID !== request.document_id || scope.browser_session_id !== request.browser_session_id || source.version !== request.observation_version || source.observedPageURL !== scope.rp_url) {
    const old = root?.ssoPlan; if (root) { root.ssoPlan = undefined; root.networkFrozen = true }
    if (old) await root.pages.get(old.child)?.page.close().catch(() => {})
    return denied()
  }
  for (const value of request.secret_values || []) if (value) root.secrets.add(value)
  root.preparedDraft = true
  let keep = false
  try {
    if (request.sso_phase === 'plan') {
      root.ssoPlan = undefined
      const live = await source.page.evaluate(() => ({ documentID: window.__butlerDocumentID, revision: window.__butlerRevision }))
      if (source.dirty || source.observedDocumentID !== live.documentID || source.observedRevision !== live.revision || source.observedPrivateURL !== source.page.url()) return denied()
      const gate = { scope, dispatch, source: source.pageID, child: '', phase: 'plan', posts: 0, rootReads: 0, hops: 0 }
      root.ssoAttempt = gate
      root.ssoGate = gate
      root.popupGate = newPopupGate(root, source, 2000)
      root.popupGate.remaining = Math.min(1, root.popupGate.remaining)
      if (root.popupGate.remaining < 1) return denied()
      const popup = source.page.waitForEvent('popup', { timeout: 6000 }).catch(() => null)
      root.networkFrozen = false
      await source.page.locator(scope.trigger_selector).click({ timeout: 2000, noWaitAfter: true })
      const page = await popup
      if (!page) return denied()
      const child = root.pageIndex.get(page) || await admitPopup(root, page)
      if (!child || gate.child !== child.pageID || child.openerID !== source.pageID) return denied()
      await page.waitForURL(u => u.origin + u.pathname === scope.idp_url, { waitUntil: 'domcontentloaded', timeout: 6000 })
      root.networkFrozen = true; root.popupGate = undefined
      if (source.documentID !== request.document_id || page.isClosed() || !ssoURLAllowed(gate, page.url())) return denied()
      await observe(child)
      const childLive = await page.evaluate(() => ({ documentID: window.__butlerDocumentID, revision: window.__butlerRevision }))
      const nativeScope = { browser_session_id: request.browser_session_id, expected_url: page.url(), target_url: scope.target_url, form_selector: scope.form_selector, username_selector: scope.username_selector, password_selector: scope.password_selector }
      const native = { browser_session_id: request.browser_session_id, expected_url: page.url(), observation_version: child.version, login_phase: 'plan', login: { scope: nativeScope, username: request.sso.username, password: request.sso.password }, secret_values: request.secret_values }
      const planned = await submitLogin(child, native, childLive)
      if (gate.violated || !planned.ok || !child.loginPlan || !planned.login_nonce) return denied()
      root.ssoPlan = { nonce: planned.login_nonce, source: source.pageID, sourceDocument: source.documentID, sourceURL: source.page.url(), child: child.pageID, childDocument: child.documentID, childURL: page.url(), scope: JSON.stringify(scope), dispatch: JSON.stringify(dispatch), native: child.loginPlan }
      keep = true
      return { ok: true, login_nonce: planned.login_nonce }
    }
    const plan = root.ssoPlan; root.ssoPlan = undefined
    if (request.sso_phase !== 'submit' || root.ssoAttempt?.violated || !plan || plan.nonce !== request.login_nonce || plan.source !== source.pageID || plan.sourceDocument !== source.documentID || plan.sourceURL !== source.page.url() || plan.scope !== JSON.stringify(scope) || plan.dispatch !== JSON.stringify(dispatch)) return denied()
    const child = root.pages.get(plan.child)
    if (!child || child.page.isClosed() || child.documentID !== plan.childDocument || child.page.url() !== plan.childURL || child.openerID !== source.pageID) return denied()
    const live = await child.page.evaluate(() => ({ documentID: window.__butlerDocumentID, revision: window.__butlerRevision }))
    if (live.documentID !== plan.native.documentID || live.revision !== plan.native.revision) return denied()
    child.loginPlan = undefined
    const gate = { scope, dispatch, source: source.pageID, child: child.pageID, phase: 'submit', posts: 0, rootReads: 0, hops: 0, chain: new Set(), body: new URLSearchParams(plan.native.entries).toString() }
    root.ssoGate = gate; root.networkFrozen = false
    const settled = Promise.any([
      child.page.waitForEvent('close', { timeout: 12000 }).then(() => true),
      child.page.waitForURL(u => u.origin === origin(scope.rp_url), { waitUntil: 'domcontentloaded', timeout: 12000 }).then(() => true),
      source.page.waitForURL(scope.verify_url, { waitUntil: 'domcontentloaded', timeout: 12000 }).then(() => true),
      child.page.waitForResponse(r => r.url() === scope.target_url && r.request().method() === 'POST' && r.status() >= 400, { timeout: 12000 }).then(() => false)
    ]).then(value => value, () => false)
    await child.page.evaluate(({ scope, username, password }) => {
      const select = selector => {
        const indexed = /^(.*) >> nth=(0|[1-9][0-9]{0,2})$/.exec(selector), css = indexed ? indexed[1] : selector
        if (css.includes('>>')) return null
        const matches = document.querySelectorAll(css)
        return !indexed && matches.length !== 1 ? null : matches[indexed ? Number(indexed[2]) : 0]
      }
      const form = select(scope.form_selector), user = select(scope.username_selector), pass = select(scope.password_selector)
      if (!(form instanceof HTMLFormElement) || !(user instanceof HTMLInputElement) || !(pass instanceof HTMLInputElement) || user.form !== form || pass.form !== form || form.action !== scope.target_url) throw new Error('changed SSO form')
      const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set
      set.call(user, username); set.call(pass, password)
      HTMLFormElement.prototype.submit.call(form)
    }, request.sso)
    if (!await settled || root.ssoAttempt?.violated || gate.posts !== 1) return denied()
    gate.phase = 'verify'
    if (!child.page.isClosed()) await child.page.close()
    // The RP's postMessage navigation can race the child's close/callback
    // event. Let that exact bounded GET settle before starting our explicit
    // verification read; never retry or repeat the credential POST.
    await source.page.waitForURL(scope.verify_url, { waitUntil: 'domcontentloaded', timeout: 1500 }).catch(() => {})
    if (source.page.url() !== scope.verify_url) await source.page.goto(scope.verify_url, { waitUntil: 'domcontentloaded', timeout: 8000 })
    root.networkFrozen = true
    const observed = await observe(source)
    return { ...observed, login_posts: gate.posts, login_cookies: await root.context.cookies(scope.verify_url) }
  } catch { return denied() }
  finally {
    root.networkFrozen = true; root.allowWriteRequests = false; root.popupGate = undefined
    const childID = root.ssoGate?.child || root.ssoPlan?.child
    root.ssoGate = undefined
    if (!keep && childID) await root.pages.get(childID)?.page.close().catch(() => {})
    if (!keep) root.ssoAttempt = undefined
  }
}

async function pageReports(root, refreshAll = false) {
  const reports = []
  for (const view of root.pages.values()) {
    if (!view.page.isClosed() && (refreshAll || view.version === 0)) {
      if (origin(view.page.url()) === root.initialOrigin) {
        try { await observe(view) } catch {
          // Never stamp an earlier document as fresh after owner return or a
          // failed refresh. Retain the identity, but retire its live executor.
          await view.page.close().catch(() => {})
        }
      }
    }
    reports.push({ page_id: view.pageID, document_id: view.observedPageDocumentID || '', runtime_version: view.version,
      opener_page_id: view.openerID, url: view.observedPageURL || '', state: view.page.isClosed() ? 'closed' : view.observedPageDocumentID && view.version > 0 ? 'ready' : 'pending' })
  }
  return reports
}

async function execute(request) {
  const root = sessions.get(request.browser_session_id)
  const view = root?.managed ? root.pages.get(request.page_id) : undefined
  const gesture = root?.managed && ['browser.click', 'browser.fill', 'browser.type'].includes(request.tool_name)
  let result
  try {
    result = request.tool_name === 'browser.sso_login' ? await executeSSO(root, request) :
      request.tool_name === 'browser.sso_inspect' ? await inspectSSO(root, request) : await executeSingle(request)
    // Admission remains inside the same finite approved gesture. Delayed
    // popups outside this window are closed before any initial request.
    if (gesture && result.ok) {
      root.networkFrozen = false
      await new Promise(resolve => setTimeout(resolve, 250))
      root.popupGate = undefined
      await Promise.all([...root.pages.values()].filter(page => page !== view && !page.page.isClosed()).map(page => page.page.waitForLoadState('domcontentloaded', { timeout: 1500 }).catch(() => {})))
    }
  } finally {
    if (gesture) { root.popupGate = undefined; root.writePage = undefined; root.allowWriteRequests = false; root.networkFrozen = true }
  }
  const current = sessions.get(request.browser_session_id)
  if (current?.managed && result.ok && request.tool_name !== 'owner.control') {
    result.private_pages = await pageReports(current, request.tool_name === 'browser.pages')
    result.selected_page_id = request.tool_name === 'browser.open' ? current.pageID : request.page_id || ''
  }
  return result
}

async function ownerControl(root, command) {
  let session = root
  if (root.managed) {
    if (command.operation === 'release') {
      root.ownerMode = false; root.popupGate = undefined; root.writePage = undefined
      root.networkFrozen = true; root.allowWriteRequests = false; root.frames.clear()
      for (const view of root.pages.values()) { view.dirty = true; if (view.observedPageDocumentID) view.version++ }
      await collectSecretState(root)
      return { ok: true }
    }
    session = command.page_id ? root.pages.get(command.page_id) : [...root.pages.values()].find(view => !view.page.isClosed())
    if (!session || session.page.isClosed()) {
      if (command.operation === 'frame') return { ok: true, owner_pages: [...root.pages.values()].filter(view => !view.page.isClosed()).map(view => ({ page_id: view.pageID, document_id: view.documentID, url: visibleURL(view, view.page.url()), state: view.state })) }
      return errorResult('page_lost', 'selected owner page is unavailable')
    }
    if (root.ownerPageID !== session.pageID) { root.frames.clear(); root.ownerPageID = session.pageID }
    if (command.operation === 'take') root.ownerMode = true
    if (root.ownerMode) {
      root.writePage = session.page
      root.popupGate = newPopupGate(root, session, 30000)
    }
    if (!['frame', 'take'].includes(command.operation) && command.document_id !== session.documentID) return errorResult('stale_frame', 'selected document changed')
  }
  const page = session.page
  if (command.operation === 'take') session.preparedDraft = true
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
  return { ok: true, owner_frame: { jpeg: jpeg.toString('base64'), frame_id: id, width: 1280, height: 720, url: visibleURL(session, page.url()),
    ...(root.managed ? { page_id: session.pageID, document_id: session.documentID } : {}) },
    ...(root.managed ? { owner_pages: [...root.pages.values()].filter(view => !view.page.isClosed()).map(view => ({ page_id: view.pageID, document_id: view.documentID, url: visibleURL(view, view.page.url()), state: view.state })) } : {}) }
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
  await closeSPA()
  await browser?.close().catch(() => {})
}
