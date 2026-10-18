import { expect, test } from '@playwright/test'

test('owner mobile view hides text until reveal and clears private state on proof change', async ({ page }) => {
  const marker = 'SYNTHETIC_PRIVATE_NOTIFICATION_57'
  await page.route('**/api/v1/mobile-owner/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/devices')) await route.fulfill({ json: [{ device_id: 'synthetic-device', device_name: 'Fixture phone', status: 'ACTIVE', android_sdk: 36, app_version: 'fixture', retention_days: 30, accepted_count: 3, duplicate_count: 1, rejected_count: 0 }] })
    else if (path.endsWith('/events')) await route.fulfill({ json: [{ source_id: 'synthetic-source', revision: 1, package_name: 'dev.butler.fixture', change_type: 'POSTED', status: 'quarantined', observed_at: '2026-10-18T00:00:00Z', received_at: '2026-10-18T00:00:01Z', body: marker }] })
    else await route.fulfill({ status: 204 })
  })
  await page.goto('/mobile')
  await page.getByLabel('Owner control token').fill('synthetic-owner-proof-that-is-not-a-real-token')
  await page.getByRole('button', { name: 'Load devices' }).click()
  await expect(page.getByText('Fixture phone · ACTIVE')).toBeVisible()
  await page.getByRole('button', { name: 'Search observations' }).click()
  await expect(page.getByText(marker)).toHaveCount(0)
  await page.getByLabel('Reveal sensitive notification content').check()
  await page.getByRole('button', { name: 'Search observations' }).click()
  await expect(page.getByText(marker)).toBeVisible()
  await page.getByLabel('Owner control token').fill('different-synthetic-proof')
  await expect(page.getByText(marker)).toHaveCount(0)
  await expect(page.getByText('Fixture phone · ACTIVE')).toHaveCount(0)
})

test('noise rules require reveal, can be disabled and clear when owner proof changes', async ({ page }) => {
  const marker = 'SYNTHETIC_PRIVATE_NOISE_PATTERN_57'
  let enabled = true
  await page.route('**/api/v1/mobile-owner/**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/devices')) await route.fulfill({ json: [] })
    else if (url.pathname.endsWith('/noise-rule')) {
      enabled = route.request().postDataJSON().enabled
      await route.fulfill({ status: 204 })
    } else if (url.pathname.endsWith('/attention')) {
      await route.fulfill({ json: { enabled: true, pending: 1, remembered: 2, noise: 18, rules: [{ rule_id: 'synthetic-rule', enabled, ...(url.searchParams.get('reveal') === 'true' ? { body: marker } : {}) }] } })
    } else await route.fulfill({ status: 204 })
  })
  await page.goto('/mobile')
  await page.getByLabel('Owner control token').fill('synthetic-owner-proof-that-is-not-a-real-token')
  await page.getByRole('button', { name: 'Load devices' }).click()
  await page.getByRole('button', { name: 'Refresh attention and noise rules' }).click()
  await expect(page.getByText(marker)).toHaveCount(0)
  await page.getByLabel('Reveal sensitive notification content').check()
  await page.getByRole('button', { name: 'Refresh attention and noise rules' }).click()
  await expect(page.getByText(marker)).toBeVisible()
  await page.getByRole('button', { name: 'Disable rule' }).click()
  await expect(page.getByRole('button', { name: 'Enable rule' })).toBeVisible()
  expect(enabled).toBe(false)
  await page.getByLabel('Owner control token').fill('different-synthetic-proof')
  await expect(page.getByText(marker)).toHaveCount(0)
})
