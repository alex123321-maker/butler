import { test, expect } from '@playwright/test'
import { fixture, connect, ownerToken } from './mind-fixture'

test('owner diagnostics keep an unknown Telegram delivery separate from Web and questions', async ({ page }) => {
  await fixture(page, { tasks: false })
  await page.route('**/api/v3/communications/profile', route => route.fulfill({ json: { ready: true, binding_id: 'web-test', binding_version: 1 } }))
  await page.route('**/api/v3/communications?*', route => route.fulfill({ json: { items: [], messages: [], cursor: 0, has_more: false } }))
  await page.route('**/api/v3/communications/events?*', route => route.fulfill({ contentType: 'text/event-stream', body: 'id: 0\nevent: refresh\ndata: {}\n\n' }))
  let unavailable = false
  await page.route('**/api/v3/communications/diagnostics', route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${ownerToken}`)
    expect(new URL(route.request().url()).search).toBe('')
    if (unavailable) return route.fulfill({ status: 503, json: { error: 'communication_observation_unavailable' } })
    return route.fulfill({ json: {
      status: 'degraded', full_v1_qualified: false, drain_only: true,
      checks: [{ name: 'communication.report', status: 'degraded' }],
      deliveries: [{ channel: 'web', state: 'result_ready', count: 1 }, { channel: 'telegram', state: 'delivery_unknown', count: 1 }],
      obligations: [{ state: 'closed_with_failure', count: 1 }], interactions: [{ state: 'open', count: 1 }],
      bindings: [{ binding_id: 'web-test', channel: 'web', enabled: true, current: true }, { binding_id: 'tg', channel: 'telegram', enabled: false, current: false }],
      inputs: [], inputs_truncated: false, legacy_deliveries: [{ channel: 'telegram', state: 'pending', count: 2 }],
    } })
  })
  await connect(page)
  await page.getByText('Состояние общения', { exact: true }).click()
  await expect(page.getByText('Telegram · результат доставки неизвестен · 1', { exact: true })).toBeVisible()
  await expect(page.getByText('Веб · доступно в вебе · 1', { exact: true })).toBeVisible()
  await expect(page.getByText('Вопрос: ожидается ответ · 1', { exact: true })).toBeVisible()
  await expect(page.getByText('ответ не завершён из-за ошибки · 1', { exact: true })).toBeVisible()
  await expect(page.getByText('Telegram · отключён', { exact: true })).toBeVisible()
  unavailable = true
  await page.getByRole('button', { name: 'Обновить состояние', exact: true }).click()
  await expect(page.getByText('Состояние общения сейчас недоступно. Результат проверки неизвестен.', { exact: true })).toBeVisible()
  await expect(page.getByText('Веб · доступно в вебе · 1', { exact: true })).toHaveCount(0)
})
