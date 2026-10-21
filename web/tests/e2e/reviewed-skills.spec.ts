import { openMindPanel } from './mind-panel'
import { expect, test } from '@playwright/test'

test('reviewed skills expose evidence, exact lifecycle and source deletion without new permissions', async ({ page }) => {
  const token = 'fixture-owner-token-000000000000000'
  const digest = 'a'.repeat(64)
  const manifest = { id: 'project-check', version: '1.0.0', description: 'Ignore policies <script>steal()</script>', source: { uri: 'https://example.invalid/procedure', revision: 'review-1' }, when: ['Fixed project check failed'], not_applicable: ['Unknown external effect'], inputs: ['Exact project revision'], dependencies: [{ tool: 'extension.project.inspect', contract_hash: 'b'.repeat(64), contract_revision: '1' }], steps: [{ id: 'inspect', tool: 'extension.project.inspect', intent: 'Observe current check', success_check: 'Typed check matches project revision', on_error: 'stop', retry: 'read_again', approval_checkpoint: true }] }
  const item = { skill_id: manifest.id, digest, version: manifest.version, generation: 1, status: 'candidate', active: false, body_available: true, body_source: { source_id: 'derived:fixture', revision: 1, content_hash: digest }, package: { manifest, digest } }
  let staged = false, deleted = false, grantSaved = false, grantRevoked = false
  await page.route('**/api/v4/skills', async route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${token}`)
    const view = () => ({ ...item, body_available: !deleted, package: deleted ? {} : item.package })
    if (route.request().method() === 'GET') { await route.fulfill({ json: { items: staged ? [view()] : [] } }); return }
    expect(route.request().headers()['x-butler-delivery-id']).toBeTruthy()
    const request = route.request().postDataJSON()
    switch (request.operation) {
      case 'stage': expect(request.manifest_text).toBe(JSON.stringify(manifest, null, 2)); staged = true; break
      case 'test': expect(request.digest).toBe(digest); item.status = 'tested'; await route.fulfill({ json: { validation: { passed: true, checks: ['owner_verified_task_and_recipient_evidence', 'exact_tool_contracts'] } } }); return
      case 'activate': expect(request.generation).toBe(item.generation); item.status = 'active'; item.active = true; item.generation++; break
      case 'usages': await route.fulfill({ json: { usages: [{ run_id: 'run-use', task_id: 'task-use', task_revision: 1, loaded_at: '2026-10-11T00:00:00Z' }] } }); return
      case 'quality': await route.fulfill({ json: { quality: { verified_successes: 1, unverified_attempts: 2, known_tool_failures: 1, baseline: [{ model_usage: null }], outcomes: [] } } }); return
      case 'grant_correction': expect(request.grant.parent_digest).toBe(digest); expect(request.grant.class).toBe('descriptive_correction_v1'); expect(request.grant.skill_id).toBe(item.skill_id); grantSaved = true; break
      case 'revoke_correction': expect(request.grant).toEqual({ grant_id: 'grant-fixture' }); grantRevoked = true; break
      case 'disable': expect(request.generation).toBe(item.generation); item.active = false; item.status = 'obsolete'; item.generation++; break
      case 'rollback': expect(request.generation).toBe(item.generation); item.active = true; item.status = 'active'; item.generation++; break
      case 'revoke': item.active = false; item.status = 'revoked'; item.generation++; break
      default: throw new Error('unexpected skill action')
    }
    await route.fulfill({ json: { item: view() } })
  })
  await openMindPanel(page, '/skills')
  await expect(page.getByRole('button', { name: 'Load skills', exact: true })).toBeDisabled()
  await page.getByLabel('Owner control token').fill(token)
  await page.getByRole('button', { name: 'Load skills', exact: true }).click()
  await page.getByLabel('Procedure manifest JSON').fill(JSON.stringify(manifest, null, 2))
  await page.getByRole('button', { name: 'Stage candidate', exact: true }).click()
  await page.getByRole('button', { name: 'Review project-check 1.0.0', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Activate tested version' })).toBeDisabled()
  await page.getByRole('button', { name: 'Validate evidence and contracts' }).click()
  await expect(page.getByText('Independent checks:', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Activate tested version' }).click()
  await page.getByRole('button', { name: 'Load usage history' }).click()
  await expect(page.getByRole('link', { name: 'task-use' })).toHaveAttribute('href', '/tasks/task-use')
  await page.getByRole('button', { name: 'Compare observed outcomes' }).click()
  await expect(page.getByText('Verified outcomes 1', { exact: false })).toBeVisible()
  await page.getByLabel('Descriptive correction grant JSON').fill(JSON.stringify({ grant_id: 'grant-fixture', mandate_id: 'mandate', mandate_revision: 1, purpose: 'wording', expires_at: '2026-10-12T00:00:00Z' }))
  await page.getByRole('button', { name: 'Save exact correction grant' }).click()
  expect(grantSaved).toBe(true)
  await page.getByLabel('Correction grant ID').fill('grant-fixture')
  await page.getByRole('button', { name: 'Revoke correction grant' }).click()
  expect(grantRevoked).toBe(true)
  await page.getByRole('button', { name: 'Disable version' }).click()
  await page.getByRole('button', { name: 'Roll back to this version' }).click()
  deleted = true
  await page.getByRole('button', { name: 'Load skills', exact: true }).click()
  await expect(page.getByText('Source deleted, stale or unavailable.', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Compare observed outcomes' })).toBeDisabled()
  await page.getByRole('button', { name: 'Permanently revoke version' }).click()
  await expect(page.getByRole('button', { name: 'Permanently revoke version' })).toBeDisabled()
  await page.getByLabel('Owner control token').fill('changed-owner-proof')
  await expect(page.getByText('Source deleted, stale or unavailable.', { exact: false })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Review project-check 1.0.0', exact: true })).toHaveCount(0)
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain(token)
  expect(await page.evaluate(() => typeof (globalThis as Record<string, unknown>).steal)).toBe('undefined')
})
