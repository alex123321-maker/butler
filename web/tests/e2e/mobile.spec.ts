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
