import { openMindPanel } from './mind-panel'
import { expect, test } from '@playwright/test'

// This opt-in fixture uses the actual Go HTTP handlers and disposable PostgreSQL.
// Only the starting run/effect observations are seeded; no provider is present.
test('actual owner API preserves branch boundaries and recovers a committed lost acknowledgement', async ({ page, request }) => {
  test.skip(process.env.BUTLER_OWNER_REAL_API !== '1', 'isolated actual owner API required')
  const api = process.env.BUTLER_API_BASE_URL!
  const state = await (await request.get(`${api}/fixture/state`)).json() as Record<string, string>
  const token = 'fixture-owner-token-000000000000000'
  await openMindPanel(page, `/tasks/${state.root}`,undefined,token)
  await expect(page.getByText('Task version 1 · waiting_for_user · normal', { exact: true })).toBeVisible()
  await page.getByLabel('Owner control token').fill(token)
  await page.getByRole('button', { name: 'Pause task', exact: true }).click()
  await expect(page.getByText('Task version 2 · paused · normal', { exact: true })).toBeVisible()
  await openMindPanel(page,new URL(page.url()).pathname,undefined,token)
  await expect(page.getByText('Task version 2 · paused · normal', { exact: true })).toBeVisible()
  await page.getByLabel('Owner control token').fill(token)
  await page.getByRole('button', { name: 'Resume task', exact: true }).click()
  await expect(page.getByText('Task version 3 · active · normal', { exact: true })).toBeVisible()
  await page.getByLabel('Amendment', { exact: true }).fill('Keep only the owner reviewed preparation')
  await page.getByLabel('Only prepare; deny external changes').check()
  await page.getByRole('button', { name: 'Apply amendment', exact: true }).click()
  await expect(page.getByText('Task version 4 · active · prepare_only', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Load task observations' }).click()
  await page.getByRole('button', { name: `Stop branch ${state.child}`, exact: true }).click()
  await expect(page.getByText('Task version 4 · active · prepare_only', { exact: true })).toBeVisible()

  await openMindPanel(page, `/tasks/${state.result}`,undefined,token)
  await page.getByLabel('Owner control token').fill(token)
  await page.getByRole('button', { name: 'Load task observations' }).click()
  await expect(page.getByText('Effect confirmed', { exact: false })).toBeVisible()
  await expect(page.getByText('Effect unknown; reconcile before repeating', { exact: false })).toBeVisible()
  await expect(page.getByText('notification delivery_unknown', { exact: false })).toBeVisible()
  await expect(page.getByRole('link', { name: `artifact ${state.artifact}`, exact: true }).first()).toHaveAttribute('href', `/artifacts?artifact_id=${encodeURIComponent(state.artifact)}`)

  await openMindPanel(page, `/tasks/${state.lost}`,undefined,token)
  await page.getByLabel('Owner control token').fill(token)
  let delivery = ''
  let body = ''
  let sends = 0
  await page.route(`**/api/v3/tasks/${state.lost}/control`, async route => {
    sends++
    if (sends === 1) {
      delivery = route.request().headers()['x-butler-delivery-id']!
      body = route.request().postData()!
      const committed = await route.fetch()
      expect(committed.status()).toBe(200)
      await route.abort('connectionclosed')
    } else {
      expect(route.request().headers()['x-butler-delivery-id']).toBe(delivery)
      expect(route.request().postData()).toBe(body)
      await route.continue()
    }
  })
  await page.getByLabel('Amendment', { exact: true }).fill('One actual durable amendment')
  await page.getByRole('button', { name: 'Apply amendment', exact: true }).click()
  await expect(page.getByText('Control outcome is unavailable.', { exact: false })).toBeVisible()
  await page.getByLabel('Amendment', { exact: true }).fill('Different unsubmitted later intent')
  await page.getByRole('button', { name: 'Retry same control', exact: true }).click()
  await expect(page.getByText('Task version 2 · active · normal', { exact: true })).toBeVisible()
  expect(sends).toBe(2)
  const observed = await request.get(`${api}/api/v3/tasks/${state.lost}/observations`, { headers: { Authorization: `Bearer ${token}` } })
  expect(observed.status()).toBe(200)
  expect((await observed.json()).revision).toBe(2)
  expect((await request.post(`${api}/fixture/done`)).status()).toBe(200)
})
