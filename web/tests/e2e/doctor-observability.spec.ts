import { expect, test } from '@playwright/test'

test('owner Doctor displays partial observations, approvals and historical reports', async ({ page }) => {
  const historical = { id: 7, status: 'healthy', checked_at: '2020-01-01T00:00:00Z', report: { status: 'healthy', checks: [{ name: 'historical_config', status: 'healthy' }] } }
	let saved: Record<string, unknown> | null = null
  const memory = {
    status: 'unknown', checked_at: new Date().toISOString(),
    components: {
      legacy_pipeline: { configured: false, effective: false, status: 'disabled', reason: 'not_configured', checked_at: new Date().toISOString() },
      semantic_retrieval: { configured: true, effective: true, status: 'unknown', reason: 'not_observed', checked_at: new Date().toISOString() },
    },
    legacy_queue: { status: 'failed', reason: 'backend' }, jobs: { status: 'unknown' }, indexes: { status: 'healthy', generations: [] },
    answer_quality: { status: 'unverified', reason: 'no_applicable_answer_quality_report', configuration: 'fixture-configuration' },
  }
  await page.route('**/api/v1/doctor/reports', async route => {
    expect(route.request().headers().authorization).toBe('Bearer owner-fixture')
    await route.fulfill({ json: { reports: saved ? [saved, historical] : [historical] } })
  })
  await page.route('**/api/v1/doctor/check', async route => {
    expect(route.request().headers().authorization).toBe('Bearer owner-fixture')
    const body = route.request().postDataJSON()
    const report = body.input_id ? { status: 'unhealthy', input_id: 'diagnostic-input', check_error: 'tool_failed', checks: [], memory, execution: { status: 'completed', run_id: 'run-diagnostic' } }
      : { status: 'unknown', input_id: 'diagnostic-input', checks: [], memory, execution: { status: 'waiting_for_user', run_id: 'run-diagnostic' } }
	 saved = { id: 8, status: report.status, checked_at: new Date().toISOString(), report }
    await route.fulfill({ status: body.input_id ? 200 : 202, json: saved })
  })
  await page.goto('/doctor')
  await expect(page.getByRole('button', { name: 'Run System Check' })).toBeDisabled()
  await page.getByLabel('Owner control token').fill('owner-fixture')
  await page.getByRole('button', { name: 'Load reports' }).click()
  await expect(page.getByText('historical_config')).toBeVisible()
  await page.getByRole('button', { name: 'Run System Check' }).click()
  await expect(page.getByRole('link', { name: 'Review the exact action in Approvals' })).toBeVisible()
	await page.reload()
	await page.getByLabel('Owner control token').fill('owner-fixture')
	await page.getByRole('button', { name: 'Load reports' }).click()
	await expect(page.getByRole('link', { name: 'Review the exact action in Approvals' })).toBeVisible()
  await expect(page.getByText('legacy_pipeline', { exact: true })).toBeVisible()
  await expect(page.getByText('semantic_retrieval', { exact: true })).toBeVisible()
  await expect(page.getByText('Answer quality: unverified', { exact: false })).toBeVisible()
  await expect(page.getByText('Legacy Redis queue: failed · depth unknown')).toBeVisible()
  await page.getByRole('button', { name: 'Refresh check' }).click()
  await expect(page.getByRole('alert')).toContainText('tool_failed')
  await page.screenshot({ path: 'test-results/doctor-observability.png', fullPage: true })
  await page.getByLabel('Owner control token').fill('')
  await expect(page.getByText('fixture-configuration', { exact: false })).toHaveCount(0)
})

test('changing owner proof discards an outstanding diagnostic response', async ({ page }) => {
  let release: () => void = () => {}
  const wait = new Promise<void>(resolve => { release = resolve })
  let requested = false
  await page.route('**/api/v1/doctor/check', async route => {
    requested = true
    await wait
    await route.fulfill({ json: { id: 0, status: 'unknown', report: { status: 'unknown', checks: [{ name: 'PRIVATE_OLD_REPORT', status: 'healthy' }] } } })
  })
  await page.goto('/doctor')
  await page.getByLabel('Owner control token').fill('first-owner')
  await page.getByRole('button', { name: 'Run System Check' }).click()
  await expect.poll(() => requested).toBeTruthy()
  await page.getByLabel('Owner control token').fill('second-owner')
  release()
  await expect(page.getByRole('button', { name: 'Run System Check' })).toBeEnabled()
  await expect(page.getByText('PRIVATE_OLD_REPORT')).toHaveCount(0)
})
