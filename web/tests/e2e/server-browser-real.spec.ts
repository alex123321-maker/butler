import { expect, test } from '@playwright/test'

test('actual owner Web API -> Broker -> PostgreSQL -> server Chromium viewing and manual input', async ({ page }) => {
  const endpoint = process.env.BUTLER_SERVER_BROWSER_REAL_API
  test.skip(!endpoint, 'Requires the isolated real Broker/PG/Chromium fixture')
  const token = 'owner-browser-fixture-'.repeat(2)
  // Fixed loopback test ingress; responses come from the production Core owner
  // handler and the actual private Broker/runtime, with no mocked reply JSON.
  await page.route('**/api/v4/server-browser', async route => {
    const response = await route.fetch({ url: `${endpoint}/api/v4/server-browser` })
    await route.fulfill({ response })
  })
  await page.goto('/browser')
  await page.getByLabel('Owner control token').fill(token)
  await page.getByRole('button', { name: 'Load browsers' }).click()
  const image = page.getByAltText('Live server browser page')
  await expect(image).toBeVisible()
  await page.getByRole('button', { name: 'Take control' }).click()
  await expect(page.getByText('You control this browser. Agent browser actions are paused.')).toBeVisible()
  let box = await image.boundingBox()
  expect(box).toBeTruthy()
  await image.click({ position: { x: 40 * box!.width / 1280, y: 120 * box!.height / 720 } })
  await page.getByLabel('Remote text').fill('owner-web-private-password')
  await page.getByRole('button', { name: 'Send text' }).click()
  await expect(page.getByLabel('Remote text')).toHaveValue('')
  await expect(page.getByRole('button', { name: 'Return to agent' })).toBeEnabled()
  box = await image.boundingBox()
  await image.click({ position: { x: 50 * box!.width / 1280, y: 200 * box!.height / 720 } })
  await expect(page.getByRole('button', { name: 'Return to agent' })).toBeEnabled()
  await expect(page.getByText('Browser unavailable or page changed.', { exact: false })).toHaveCount(0)
  await page.screenshot({ path: 'test-results/server-browser-actual.png', fullPage: true })
  await page.getByRole('button', { name: 'Return to agent' }).click()
  await expect(page.getByText('Viewing agent actions.')).toBeVisible()
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toMatch(/owner-web-private-password|owner-browser-fixture/)
})
