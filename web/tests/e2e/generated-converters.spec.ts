import { openMindPanel } from './mind-panel'
import { expect, test } from '@playwright/test'
import { createHash } from 'node:crypto'

test('generated converter review keeps owner proof, exact version rights and current output integrity', async ({ page }) => {
  const token = 'fixture-owner-converter-token-000000000000'
  const body = '[{"label":"<script>steal()</script>","quantity":2}]'
  const ref = { source_id: 'source_output', revision: 1, access_revision: 1, content_hash: 'd'.repeat(64) }
  const entry = { record: { converter_id: 'inventory', version: '1.0.0', digest: 'a'.repeat(64), need_hash: 'b'.repeat(64), profile_pin: 'c'.repeat(64), generation: 2, status: 'active', active: true, permission: { converter_id: 'inventory', class: 'delimited_json_v1', input_schema_hash: 'e'.repeat(64), output_schema_hash: 'f'.repeat(64), runtime_pin: 'c'.repeat(64), input_bytes: 8192, output_bytes: 32768, rows: 128, field_bytes: 1024, millis: 1000, trials: 2 } }, checks: { passed: true, checker: 'butler.delimited-json.checks.v1', checks: ['independent need mapping', 'no network or process operations'] }, trials: 1, passed_trials: 1, uses: 1, outputs: [{ run_id: 'run-convert', task_id: 'task-convert', task_revision: 1, generation: 2, source_ref: ref, sha256: createHash('sha256').update(body).digest('hex') }] }
  let deleted = false
  await page.route('**/api/v4/converters', async route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${token}`)
    if (route.request().method() === 'GET') return route.fulfill({ json: { items: [entry] } })
    expect(route.request().headers()['x-butler-delivery-id']).toBeTruthy()
    expect(route.request().postDataJSON()).toEqual({ operation: 'revoke', digest: entry.record.digest, generation: 2 })
    entry.record.status = 'revoked'; entry.record.active = false; entry.record.generation++
    return route.fulfill({ json: { item: entry.record } })
  })
  await page.route('**/api/v4/memory/source?**', async route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${token}`)
    expect(new URL(route.request().url()).searchParams.get('source_id')).toBe(ref.source_id)
    return route.fulfill({ json: { source: { ...ref, status: deleted ? 'deleted' : 'active', body: deleted ? '' : body } } })
  })
  await openMindPanel(page, '/converters')
  await expect(page.getByRole('button', { name: 'Load converters' })).toBeDisabled()
  await page.getByLabel('Owner control token').fill(token)
  await page.getByRole('button', { name: 'Load converters' }).click()
  await expect(page.getByText('Working credential, file, network and process access: none.', { exact: false })).toBeVisible()
  await expect(page.getByText('Trials 1/2', { exact: false })).toBeVisible()
  await expect(page.getByRole('link', { name: 'task-convert' })).toHaveAttribute('href', '/tasks/task-convert')
  await page.getByRole('button', { name: 'Review checked output' }).click()
  await expect(page.locator('pre')).toHaveText(body)
  expect(await page.evaluate(() => typeof (globalThis as Record<string, unknown>).steal)).toBe('undefined')
  deleted = true
  await page.getByRole('button', { name: 'Review checked output' }).click()
  await expect(page.getByText('Output deleted, changed or unavailable.', { exact: false })).toBeVisible()
  await expect(page.locator('pre')).toHaveCount(0)
  await page.getByRole('button', { name: 'Permanently revoke inventory 1.0.0' }).click()
  await expect(page.getByRole('button', { name: 'Permanently revoke inventory 1.0.0' })).toBeDisabled()
  await page.getByLabel('Owner control token').fill('new-owner-proof')
  await expect(page.getByRole('button', { name: 'Review checked output' })).toHaveCount(0)
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain(token)
})
