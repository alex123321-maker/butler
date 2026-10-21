import { openMindPanel } from './mind-panel'
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
  await openMindPanel(page,'/browser',undefined,token)
  const image = page.getByAltText('Страница серверного браузера')
  await expect(image).toBeVisible()
  await expect(page.getByText('http://login.test/form')).toBeVisible()
  await page.getByRole('button', { name: 'Взять управление' }).click()
  await expect(page.getByText('Вы управляете · агент ждёт')).toBeVisible()
  let box = await image.boundingBox()
  expect(box).toBeTruthy()
  await image.click({ position: { x: 40 * box!.width / 1280, y: 120 * box!.height / 720 } })
  await page.getByLabel('Текст для страницы').fill('owner-web-private-password')
  await page.getByRole('button', { name: 'Ввести текст' }).click()
  await expect(page.getByLabel('Текст для страницы')).toHaveValue('')
  await expect(page.getByRole('button', { name: 'Вернуть агенту' })).toBeEnabled()
  box = await image.boundingBox()
  await image.click({ position: { x: 50 * box!.width / 1280, y: 200 * box!.height / 720 } })
  await expect(page.getByRole('button', { name: 'Вернуть агенту' })).toBeEnabled()
  await expect(page.getByText('Команда не подтверждена или страница изменилась.', { exact: false })).toHaveCount(0)
  await page.screenshot({ path: 'test-results/server-browser-actual.png', fullPage: true })
  await page.getByRole('button', { name: 'Вернуть агенту' }).click()
  await expect(page.getByText('Управляет агент')).toBeVisible()
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toMatch(/owner-web-private-password|owner-browser-fixture/)
})
