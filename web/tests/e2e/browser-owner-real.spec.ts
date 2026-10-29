import { expect, test } from '@playwright/test'
import { openMindPanel } from './mind-panel'

test('owner UI stop and mandate revoke fence already authorized real browser POSTs', async ({ page, request }) => {
  test.skip(process.env.BUTLER_BROWSER_OWNER_UI_E2E !== '1', 'isolated real owner API/Broker/Chromium required')
  test.setTimeout(180000)
  const coordination = 'http://127.0.0.1:4182'
  const token = 'fixture-owner-token-000000000000000'
  let api = ''
  // The response comes from actual Go owner handlers, not fixture JSON. The
  // shared helper mocks unrelated navigation only and removes those routes.
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url())
    const response = await route.fetch({ url: `${api}${url.pathname}${url.search}` })
    await route.fulfill({ response })
  })
  for (const kind of ['stop', 'revoke']) {
    await expect.poll(async () => {
      try { return (await (await request.get(`${coordination}/state`)).json()).kind }
      catch { return '' }
    }, { timeout: 60000 }).toBe(kind)
    const state = await (await request.get(`${coordination}/state`)).json() as { api: string; task: string }
    api = state.api
    const started = Date.now()
    await openMindPanel(page, kind === 'stop' ? `/tasks/${state.task}` : '/goals', undefined, token)
    await page.getByLabel('Owner control token', { exact: true }).fill(token)
    if (kind === 'stop') {
      await page.getByRole('button', { name: 'Stop task', exact: true }).click()
      await expect(page.getByText('Task version 2 · cancelled · normal', { exact: true })).toBeVisible()
    } else {
      await page.getByRole('button', { name: 'Load owner authority', exact: true }).click()
      // The stop case has its own mandate, so target the mandate assigned to
      // this task from the actual service view instead of relying on order.
      const refs = await (await request.get(`${api}/fixture/state`)).json() as Record<string, string>
      const authority = await (await request.get(`${api}/api/v3/owner-authority`, { headers: { Authorization: `Bearer ${token}` } })).json() as { mandates: { mandate_id: string }[] }
      const index = authority.mandates.findIndex(mandate => mandate.mandate_id === refs.revoke_mandate)
      expect(index).toBeGreaterThanOrEqual(0)
      const card = page.locator('.authority-row').filter({ has: page.getByRole('button', { name: 'Revoke mandate', exact: true }) }).nth(index)
      await card.getByRole('button', { name: 'Revoke mandate', exact: true }).click()
      await expect(page.getByText('New actions under old authority are already denied.', { exact: false })).toBeVisible()
    }
    expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain(token)
    expect((await request.post(`${coordination}/continue?kind=${kind}`)).status()).toBe(200)
    console.log(JSON.stringify({ action: `owner_ui_${kind}`, wall_ms: Date.now() - started, model_calls: 0 }))
  }
  await expect.poll(async () => (await (await request.get(`${coordination}/state`)).json()).kind).toBe('done')
  expect((await request.post(`${coordination}/continue?kind=done`)).status()).toBe(200)
})
