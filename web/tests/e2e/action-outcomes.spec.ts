import { expect, test } from '@playwright/test'
import { connect, fixture, ownerToken } from './mind-fixture'

test.use({ screenshot: 'off', trace: 'off', video: 'off' })

test('owner reports a factual outcome with separate receipt and no automatic replay', async ({ page }) => {
  await fixture(page, { tasks: false })
  let reports = 0, pending = true
  await page.route('**/api/v4/action-outcomes', async route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${ownerToken}`)
    if (route.request().method() === 'GET') {
      await route.fulfill({ json: { reviews: pending ? [{ review_id: 'review-one', effect_id: 'effect-one', fingerprint: 'a'.repeat(64), revision: 1, resource_hint: 'https://example.com/form', expires_at: '2099-01-01T00:00:00Z' }] : [] } })
      return
    }
    reports++
    const { decision } = route.request().postDataJSON()
    expect(decision.outcome).toBe('effect_confirmed')
    expect(decision.recipient_checked).toBe(true)
    expect(decision.processing_settled).toBe(true)
    expect(decision.receipt_reference).toBe('fixture-receipt-one')
    expect(decision.approved).toBeUndefined()
    pending = false
    await route.fulfill({ json: { review: { state: 'effect_confirmed' } } })
  })
  await connect(page)
  await expect(page.getByRole('heading', { name: 'Проверьте результат действия' })).toBeVisible()
  const submit = page.getByRole('button', { name: 'Сообщить исход' })
  await expect(submit).toBeDisabled()
  await page.getByLabel('Проверенный исход').selectOption('effect_confirmed')
  await expect(submit).toBeDisabled()
  await page.getByLabel('Я проверил состояние у получателя').check()
  await page.getByLabel('Обработка завершена, отложенных операций нет').check()
  await page.getByLabel('Номер или ссылка на подтверждение, без паролей').fill('fixture-receipt-one')
  await submit.click()
  await expect(page.getByRole('heading', { name: 'Проверьте результат действия' })).toBeHidden()
  expect(reports).toBe(1)
})

test('unknown outcome stays explicit even after a known choice was drafted', async ({ page }) => {
  await fixture(page)
  let reports = 0
  let pending = true
  await page.route('**/api/v4/action-outcomes', async route => {
    if (route.request().method() === 'GET') {
      await route.fulfill({ json: { reviews: pending ? [{ review_id: 'review-unknown', fingerprint: 'b'.repeat(64), revision: 1, resource_hint: 'https://example.com/form' }] : [] } })
      return
    }
    reports++
    const { decision } = route.request().postDataJSON()
    expect(decision.outcome).toBe('unknown')
    expect(decision.recipient_checked).toBe(false)
    expect(decision.processing_settled).toBe(false)
    expect(decision.receipt_reference).toBe('')
    pending = false
    await route.fulfill({ json: { review: { state: 'unresolved' } } })
  })
  await connect(page)
  await page.getByLabel('Проверенный исход').selectOption('effect_confirmed')
  await page.getByLabel('Я проверил состояние у получателя').check()
  await page.getByLabel('Обработка завершена, отложенных операций нет').check()
  await page.getByLabel('Номер или ссылка на подтверждение, без паролей').fill('discarded-draft')
  await page.getByLabel('Проверенный исход').selectOption('unknown')
  await page.getByRole('button', { name: 'Сообщить исход' }).click()
  await expect(page.getByRole('heading', { name: 'Проверьте результат действия' })).toBeHidden()
  expect(reports).toBe(1)
})

test('lost report acknowledgement is not automatically resubmitted', async ({ page }) => {
  await fixture(page)
  let reports = 0
  await page.route('**/api/v4/action-outcomes', async route => {
    if (route.request().method() === 'GET') {
      await route.fulfill({ json: { reviews: [{ review_id: 'review-ack', fingerprint: 'c'.repeat(64), revision: 1, resource_hint: 'https://example.com/form' }] } })
      return
    }
    reports++
    await route.abort('failed')
  })
  await connect(page)
  await page.getByLabel('Проверенный исход').selectOption('unknown')
  await page.getByRole('button', { name: 'Сообщить исход' }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Ответ не подтверждён' })).toBeVisible()
  // A complete polling interval must not turn an ambiguous POST into a retry.
  await page.waitForRequest(request => request.method() === 'GET' && request.url().endsWith('/api/v4/action-outcomes'), { timeout: 4000 })
  expect(reports).toBe(1)
})
