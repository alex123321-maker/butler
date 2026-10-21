import { openMindPanel } from './mind-panel'
import { expect, test } from '@playwright/test'

test('private memory confirms scoped candidates, preserves history and deletes source content', async ({ page }) => {
  const token = 'fixture-memory-owner-token-000000000000'
  const source = { source_id: 'source-fixture', external_key: 'project-rule', source_kind: 'owner_statement', scope_type: 'project', scope_id: 'owner:butler', revision: 1, status: 'active', title: 'Правило проекта', body: 'Пиши коротко по-русски', author: 'Владелец', event_time: '2026-01-01T00:00:00Z', extracted_at: '2026-10-11T00:00:00Z', origin: { kind: 'owner_input', sensitivity: 'private', parent_refs: [] } }
  const claims = [{ claim_id: 'old', key: 'language', scope_type: source.scope_type, scope_id: source.scope_id, source_id: source.source_id, source_revision: 1, summary: 'English', value: 'en', status: 'confirmed', extraction_confidence: .99, supersedes_id: '' }, { claim_id: 'new', key: 'language', scope_type: source.scope_type, scope_id: source.scope_id, source_id: source.source_id, source_revision: 1, summary: 'Русский', value: 'ru', status: 'candidate', extraction_confidence: .5, supersedes_id: '' }]
  const revisions = [{ revision: 1, author: source.author, event_time: source.event_time, body: source.body, content_hash: 'hash-1' }]
  await page.route('**/api/v4/memory**', async route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${token}`)
    const url = new URL(route.request().url()); const path = url.pathname
    if (path === '/api/v4/memory') return route.fulfill({ json: { sources: [source], claims, source_list_limit: 1000, claim_list_limit: 2000, index_jobs: [], semantic_enabled: false } })
    if (path === '/api/v4/memory/source') return route.fulfill({ json: { source, revisions, fragments: [], assets: [], revision_limit: 5000 } })
    if (path === '/api/v4/memory/export') return route.fulfill({ headers: { 'Content-Type': 'application/x-ndjson' }, body: JSON.stringify({ record_type: 'source_revision', data: source }) })
    expect(route.request().headers()['x-butler-delivery-id']).toBeTruthy()
    const q = route.request().postDataJSON()
    if (path === '/api/v4/memory/decisions') { expect(q).toEqual({ claim_id: 'new', expected_active_id: 'old', reject: false }); claims[0]!.status = 'superseded'; claims[1]!.status = 'confirmed'; claims[1]!.supersedes_id = 'old' }
    else if (path === '/api/v4/memory/correct') { expect(q.expected_revision).toBe(1); source.body = q.body; source.revision = 2; revisions.push({ ...revisions[0]!, revision: 2, body: q.body, content_hash: 'hash-2' }) }
    else if (path === '/api/v4/memory/delete') { expect(q).toEqual({ source_id: source.source_id, expected_revision: 2, action: 'delete' }); source.status = 'deleted'; source.body = ''; revisions.forEach(r => { r.body = '' }); claims.forEach(c => { c.status = 'deleted'; c.value = ''; c.summary = '' }) }
    else throw new Error(`Unexpected ${path}`)
    return route.fulfill({ json: source })
  })
  await openMindPanel(page, '/memory')
  await expect(page.getByRole('button', { name: 'Load private memory' })).toBeDisabled()
  await page.getByLabel('Owner control token').fill(token)
  await page.getByRole('button', { name: 'Load private memory' }).click()
  await expect(page.getByText('language · candidate', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Confirm candidate' }).click()
  await expect(page.getByText('language · superseded', { exact: true })).toBeVisible()
  await expect(page.getByText('language · confirmed', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Inspect source' }).click()
  await page.getByLabel('Source text').fill('Пиши ещё короче по-русски')
  await page.getByRole('button', { name: 'Save source correction' }).click()
  await expect(page.getByText('Retained versions 2', { exact: false })).toBeVisible()
  const downloaded = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Export all memory' }).click()
  expect((await downloaded).suggestedFilename()).toBe('butler-memory.ndjson')
  await page.getByRole('button', { name: 'Delete source and derivatives' }).click()
  await expect(page.getByText('source-fixture · revision 2 · deleted', { exact: true })).toBeVisible()
  await expect(page.getByLabel('Source text')).toHaveCount(0)
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain(token)
})

test('selected Russian file opens current original context and neighbouring fragments', async ({ page }) => {
  const token = 'fixture-memory-owner-token-000000000000'
  const text = '# Русский раздел\nService.Restart требует отдельного разрешения\n'
  const source = { source_id: 'uploaded', external_key: 'document-logical-id', source_kind: 'document', scope_type: 'user', scope_id: 'owner', revision: 1, status: 'active', title: 'русский.md', body: text, author: 'Owner', event_time: null, extracted_at: '2026-10-11T00:00:00Z' }
  let uploaded = false
  await page.route('**/api/v4/memory**', async route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${token}`)
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v4/memory') return route.fulfill({ json: { sources: uploaded ? [source] : [], claims: [], source_list_limit: 1000, claim_list_limit: 2000, index_jobs: [], semantic_enabled: false } })
    if (path === '/api/v4/memory/ingest') { const q = route.request().postDataJSON(); expect(Buffer.from(q.original_bytes, 'base64').toString('utf8')).toBe(text); expect(q.expected_revision).toBe(0); expect(q.format).toBe('markdown'); uploaded = true; return route.fulfill({ json: { source, status: 'ready', uncertain: false } }) }
    if (path === '/api/v4/memory/source') return route.fulfill({ json: { source, revisions: [], revision_limit: 5000, fragments: [{ chunk_id: 7, title: 'Русский раздел', text, locator: { path: 'русский.md', start_line: 1 } }], assets: [{ source_revision: 1, path: 'русский.md', format: 'markdown', parser_version: 'structured-go-v1', method: 'structured-go-v1', status: 'ready', uncertain: false }] } })
    if (path === '/api/v4/memory/citation') return route.fulfill({ json: { source, fragment: { Key: 'section:1', Text: text }, neighbors: [{ Key: 'next', Text: 'Соседний раздел поясняет ограничение' }] } })
    throw new Error(`Unexpected ${path}`)
  })
  await openMindPanel(page, '/memory'); await page.getByLabel('Owner control token').fill(token); await page.getByRole('button', { name: 'Load private memory' }).click()
  await page.getByLabel('Source file').setInputFiles({ name: 'русский.md', mimeType: 'text/markdown', buffer: Buffer.from(text) })
  await page.getByLabel('Stable source key').fill(source.external_key); await page.getByLabel('Source scope ID').fill('owner'); await page.getByLabel('Source format').selectOption('markdown')
  await page.getByRole('button', { name: 'Parse and save source' }).click()
  await expect(page.getByText('Source revision 1: ready', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Open fragment context' }).click()
  await expect(page.getByText('Соседний раздел поясняет ограничение')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Download original' })).toBeVisible()
})
