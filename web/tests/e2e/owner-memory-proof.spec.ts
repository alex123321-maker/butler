import { openMindPanel } from './mind-panel'
import { expect, test } from '@playwright/test'

const source = { source_id: 'target-source', external_key: 'target-source', source_kind: 'owner_statement', scope_type: 'user', scope_id: 'owner', revision: 2, status: 'active', title: 'Target evidence', body: 'EXACT_SOURCE_BODY', author: 'Owner', event_time: null, extracted_at: '2026-10-12T00:00:00Z' }
const view = { sources: [source], claims: [], source_list_limit: 1000, claim_list_limit: 2000, index_jobs: [], semantic_enabled: false }
const detail = { source, revisions: [], fragments: [], assets: [], revision_limit: 5000 }

test('same-owner source route change queues the exact latest source and never republishes delayed old body', async ({ page }) => {
  let release: () => void = () => {}
  const delayed = new Promise<void>(resolve => { release = resolve })
  let started: () => void = () => {}
  const oldStarted = new Promise<void>(resolve => { started = resolve })
  const paths: string[] = []
  await page.route('**/api/v4/memory**', async route => {
    expect(route.request().headers().authorization).toBe('Bearer current-owner-proof')
    const url = new URL(route.request().url())
    if (url.pathname !== '/api/v4/memory/source') return route.fulfill({ json: view })
    const id = url.searchParams.get('source_id')!
    paths.push(id)
    if (id === 'target-source') { started(); await delayed }
    return route.fulfill({ json: { ...detail, source: { ...source, source_id: id, body: id === 'next-source' ? 'LATEST_SOURCE_BODY' : 'PRIVATE_OLD_SOURCE_BODY' } } })
  })
  await openMindPanel(page, '/memory?source_id=target-source')
  await page.getByLabel('Owner control token').fill('current-owner-proof')
  await page.getByRole('button', { name: 'Load private memory' }).click()
  await oldStarted
  // Drive the actual mounted Vue router, preserving the page and current proof.
  await page.evaluate(async () => {
    const root = document.getElementById('__nuxt') as HTMLElement & { __vue_app__: { config: { globalProperties: { $router: { push(path: string): Promise<void> } } } } }
    await root.__vue_app__.config.globalProperties.$router.push('/memory/manage?source_id=next-source')
  })
  await expect(page).toHaveURL(/source_id=next-source/)
  await expect(page.getByLabel('Source text', { exact: true })).toHaveCount(0)
  release()
  await expect(page.getByLabel('Source text', { exact: true })).toHaveValue('LATEST_SOURCE_BODY')
  await expect(page.getByText('PRIVATE_OLD_SOURCE_BODY')).toHaveCount(0)
  expect(paths).toEqual(['target-source', 'next-source'])
})

test('exact evidence source loads only after explicit owner authentication', async ({ page }) => {
  const paths: string[] = []
  await page.route('**/api/v4/memory**', async route => {
    expect(route.request().headers().authorization).toBe('Bearer current-owner-proof')
    const url = new URL(route.request().url())
    paths.push(url.pathname)
    if (url.pathname === '/api/v4/memory/source') {
      expect(url.searchParams.get('source_id')).toBe('target-source')
      return route.fulfill({ json: detail })
    }
    return route.fulfill({ json: view })
  })
  await openMindPanel(page, '/memory?source_id=target-source')
  await expect(page.getByRole('button', { name: 'Load private memory' })).toBeDisabled()
  expect(paths).toEqual([])
  await page.getByLabel('Owner control token').fill('current-owner-proof')
  await page.getByRole('button', { name: 'Load private memory' }).click()
  await expect(page.getByLabel('Source text', { exact: true })).toHaveValue('EXACT_SOURCE_BODY')
  expect(paths).toEqual(['/api/v4/memory', '/api/v4/memory/source'])
})

test('old proof cannot republish a delayed source body', async ({ page }) => {
  let release: () => void = () => {}
  const delayed = new Promise<void>(resolve => { release = resolve })
  let started: () => void = () => {}
  const requestStarted = new Promise<void>(resolve => { started = resolve })
  await page.route('**/api/v4/memory**', async route => {
    expect(route.request().headers().authorization).toBe('Bearer previous-owner-proof')
    if (new URL(route.request().url()).pathname === '/api/v4/memory/source') {
      started()
      await delayed
      return route.fulfill({ json: { ...detail, source: { ...source, body: 'PRIVATE_OLD_SOURCE_BODY' } } })
    }
    return route.fulfill({ json: view })
  })
  await openMindPanel(page, '/memory?source_id=target-source')
  await page.getByLabel('Owner control token').fill('previous-owner-proof')
  await page.getByRole('button', { name: 'Load private memory' }).click()
  await requestStarted
  await page.getByLabel('Owner control token').fill('another-owner-proof')
  const response = page.waitForResponse('**/api/v4/memory/source?source_id=target-source')
  release()
  await (await response).finished()
  await expect(page.getByLabel('Source text', { exact: true })).toHaveCount(0)
  await expect(page.getByText('PRIVATE_OLD_SOURCE_BODY')).toHaveCount(0)
})

test('acknowledged old-proof mutation starts no followup request under a changed proof', async ({ page }) => {
  let release: () => void = () => {}
  const delayed = new Promise<void>(resolve => { release = resolve })
  let started: () => void = () => {}
  const requestStarted = new Promise<void>(resolve => { started = resolve })
  const requests: string[] = []
  await page.route('**/api/v4/memory**', async route => {
    expect(route.request().headers().authorization).toBe('Bearer previous-owner-proof')
    const path = new URL(route.request().url()).pathname
    requests.push(path)
    if (path === '/api/v4/memory/source') return route.fulfill({ json: detail })
    if (path === '/api/v4/memory/correct') { started(); await delayed; return route.fulfill({ json: { ...source, revision: 3 } }) }
    return route.fulfill({ json: view })
  })
  await openMindPanel(page, '/memory?source_id=target-source')
  await page.getByLabel('Owner control token').fill('previous-owner-proof')
  await page.getByRole('button', { name: 'Load private memory' }).click()
  await page.getByLabel('Source text', { exact: true }).fill('A reviewed correction')
  await page.getByRole('button', { name: 'Save source correction' }).click()
  await requestStarted
  await page.getByLabel('Owner control token').fill('another-owner-proof')
  const response = page.waitForResponse('**/api/v4/memory/correct')
  release()
  await (await response).finished()
  await expect(page.getByLabel('Source text', { exact: true })).toHaveCount(0)
  await expect(page.getByText('Saved with the current source revision.', { exact: false })).toHaveCount(0)
  expect(requests).toEqual(['/api/v4/memory', '/api/v4/memory/source', '/api/v4/memory/correct'])
})

test('export response from an old proof cannot start a browser download', async ({ page }) => {
  let release: () => void = () => {}
  const delayed = new Promise<void>(resolve => { release = resolve })
  let started: () => void = () => {}
  const requestStarted = new Promise<void>(resolve => { started = resolve })
  let downloads = 0
  page.on('download', () => { downloads++ })
  await page.route('**/api/v4/memory**', async route => {
    expect(route.request().headers().authorization).toBe('Bearer previous-owner-proof')
    if (new URL(route.request().url()).pathname === '/api/v4/memory/export') { started(); await delayed; return route.fulfill({ contentType: 'application/x-ndjson', body: 'PRIVATE_OLD_EXPORT' }) }
    return route.fulfill({ json: view })
  })
  await openMindPanel(page, '/memory')
  await page.getByLabel('Owner control token').fill('previous-owner-proof')
  await page.getByRole('button', { name: 'Load private memory' }).click()
  await page.getByRole('button', { name: 'Export all memory' }).click()
  await requestStarted
  await page.getByLabel('Owner control token').fill('another-owner-proof')
  const response = page.waitForResponse('**/api/v4/memory/export')
  release()
  await (await response).finished()
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
  expect(downloads).toBe(0)
  await expect(page.getByText('PRIVATE_OLD_EXPORT')).toHaveCount(0)
})

test('changed owner proof during local file reading prevents upload egress', async ({ page }) => {
  await page.addInitScript(() => {
    const original = File.prototype.arrayBuffer
    File.prototype.arrayBuffer = function () {
      const file = this
      const scope = globalThis as typeof globalThis & { ownerUploadStarted?: boolean; releaseOwnerUpload?: () => void }
      scope.ownerUploadStarted = true
      return new Promise<ArrayBuffer>(resolve => { scope.releaseOwnerUpload = () => { void original.call(file).then(resolve) } })
    }
  })
  let uploads = 0
  await page.route('**/api/v4/memory**', route => {
    if (new URL(route.request().url()).pathname === '/api/v4/memory/ingest') uploads++
    return route.fulfill({ json: view })
  })
  await openMindPanel(page, '/memory')
  await page.getByLabel('Owner control token').fill('previous-owner-proof')
  await page.getByRole('button', { name: 'Load private memory' }).click()
  await page.getByLabel('Source file').setInputFiles({ name: 'private.txt', mimeType: 'text/plain', buffer: Buffer.from('PRIVATE_UPLOAD_BODY') })
  await page.getByLabel('Stable source key').fill('private-upload')
  await page.getByLabel('Source scope ID').fill('owner')
  await page.getByRole('button', { name: 'Parse and save source' }).click()
  await page.waitForFunction(() => Boolean((globalThis as typeof globalThis & { ownerUploadStarted?: boolean }).ownerUploadStarted))
  await page.getByLabel('Owner control token').fill('another-owner-proof')
  await page.evaluate(() => { (globalThis as typeof globalThis & { releaseOwnerUpload?: () => void }).releaseOwnerUpload?.() })
  await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
  expect(uploads).toBe(0)
  await page.getByRole('button', { name: 'Load private memory' }).click()
  await expect(page.getByLabel('Stable source key')).toHaveValue('')
})
