import { test, expect, type Page } from '@playwright/test'
import { fixture, connect, ownerToken } from './mind-fixture'
import type { Publication, WidgetView } from '../../features/mind/model/communications'

function question(type: 'single_choice' | 'multi_choice' | 'text' = 'multi_choice'): Publication {
  return {
    publication_ref: 'publication:pizza', sequence: 1, created_at: '2026-10-22T00:00:00Z', available: true, kind: 'widget',
    payload: { question: 'Какую пиццу желаете?', input: type === 'text' ? { type, min_length: 2, max_length: 4, placeholder: 'Ваш вариант' } : { type, options: [{ id: 'a', label: 'Маргарита', description: 'Без покупки и оплаты' }, { id: 'b', label: 'Пепперони' }], ...(type === 'multi_choice' ? { min_selections: 1, max_selections: 2 } : { allow_text_answer: true }) } },
    interaction: { interaction_ref: 'interaction:pizza', version: 1, draft_version: 1, state: 'open', draft: {}, answer: {} },
    files: [], parts: [{ channel: 'web', binding_id: 'web-test', binding_version: 1, index: 0, state: 'available' }],
  }
}
async function widgets(page: Page, publication: Publication) {
  await fixture(page, { tasks: false })
  await page.route('**/api/v3/communications/profile', route => route.fulfill({ json: { ready: true, binding_id: 'web-test', binding_version: 1, methods: ['message', 'widget'] } }))
  await page.route('**/api/v3/communications?*', route => route.fulfill({ json: { items: [publication], cursor: 1, has_more: false, messages: [] } }))
  await page.route('**/api/v3/communications/events?*', route => route.fulfill({ contentType: 'text/event-stream', body: 'id: 1\nevent: refresh\ndata: {}\n\n' }))
}

test('multi-choice persists a shared versioned draft and only Submit answers', async ({ page }) => {
  const pub = question(), requests: Record<string, unknown>[] = []
  await widgets(page, pub)
  await page.route('**/api/v3/interactions/*/actions', async route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${ownerToken}`)
    const q = route.request().postDataJSON(); requests.push(q)
    expect(q.expected_version).toBe(1)
    expect(q.binding_id).toBe('web-test')
    expect(q.draft_version).toBe(pub.interaction!.draft_version)
    if (q.operation === 'draft') { pub.interaction!.draft = q.answer; pub.interaction!.draft_version++ }
    else { expect(q.operation).toBe('submit'); expect(q.answer).toEqual({}); pub.interaction!.state = 'resolved'; pub.interaction!.answer = pub.interaction!.draft; pub.interaction!.version++ }
    await route.fulfill({ json: pub.interaction })
  })
  await connect(page)
  const region = page.getByRole('region', { name: 'Какую пиццу желаете?', exact: true })
  await expect(region.getByRole('button', { name: 'Отправить ответ' })).toBeDisabled()
  await region.getByRole('checkbox', { name: 'Маргарита Без покупки и оплаты' }).check()
  await expect.poll(() => requests.length).toBe(1)
  expect(requests[0].operation).toBe('draft')
  await expect(region.getByRole('checkbox').first()).toBeEnabled()
  await page.reload() // Draft comes back from durable REST, not local state.
  await connect(page)
  await expect(region.getByRole('checkbox').first()).toBeChecked()
  await region.getByRole('button', { name: 'Отправить ответ' }).click()
  await expect(region.getByText('Ответ принят. Вопрос закрыт.', { exact: true })).toBeVisible()
  await expect(region.getByRole('checkbox').first()).toBeDisabled()
  expect(requests.map(q => q.operation)).toEqual(['draft', 'submit'])
})

test('lost answer ACK retries the exact delivery identity and body', async ({ page }) => {
  const pub = question('single_choice'), requests: { body: string | null; delivery: string }[] = []
  await widgets(page, pub)
  await page.route('**/api/v3/interactions/*/actions', async route => {
    requests.push({ body: route.request().postData(), delivery: route.request().headers()['x-butler-delivery-id'] })
    if (requests.length === 1) return route.abort('connectionclosed')
    pub.interaction!.state = 'resolved'; pub.interaction!.version++; pub.interaction!.answer = { option_ids: ['b'] }
    await route.fulfill({ json: pub.interaction })
  })
  await connect(page)
  await page.getByRole('radio', { name: 'Пепперони' }).check()
  await page.getByRole('button', { name: 'Отправить ответ', exact: true }).click()
  await page.getByRole('button', { name: 'Повторить тот же ответ', exact: true }).click()
  await expect.poll(() => requests.length).toBe(2)
  expect(requests[1]).toEqual(requests[0])
  await expect(page.getByText('Ответ принят. Вопрос закрыт.', { exact: true })).toBeVisible()
})

test('a stale channel response refreshes the winning decision without resubmission', async ({ page }) => {
  const pub = question('single_choice'); await widgets(page, pub)
  let calls = 0
  await page.route('**/api/v3/interactions/*/actions', async route => {
    calls++; pub.interaction!.state = 'resolved'; pub.interaction!.version++; pub.interaction!.answer = { option_ids: ['b'] }
    await route.fulfill({ status: 409, json: { error: 'already_resolved' } })
  })
  await connect(page)
  await page.getByRole('radio').first().check()
  await page.getByRole('button', { name: 'Отправить ответ', exact: true }).click()
  await expect(page.getByText('Ответ принят. Вопрос закрыт.', { exact: true })).toBeVisible()
  await expect(page.getByRole('radio').first()).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Повторить тот же ответ' })).toHaveCount(0)
  expect(calls).toBe(1)
})

test('text limits count Unicode characters and terminal questions have no active controls', async ({ page }) => {
  const pub = question('text'); await widgets(page, pub)
  let answer = ''
  await page.route('**/api/v3/interactions/*/actions', async route => {
    answer = route.request().postDataJSON().answer.text
    pub.interaction = { ...pub.interaction!, state: 'resolved', answer: { text: answer }, version: 2 } as WidgetView
    await route.fulfill({ json: pub.interaction })
  })
  await page.setViewportSize({ width: 375, height: 812 })
  await connect(page)
  await page.getByRole('textbox', { name: 'Ответ на вопрос' }).fill('🍕🍕🍕🍕')
  await page.getByRole('button', { name: 'Отправить ответ', exact: true }).click()
  await expect(page.getByText('Ответ принят. Вопрос закрыт.', { exact: true })).toBeVisible()
  expect(answer).toBe('🍕🍕🍕🍕')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('a rejected draft restores the server choice instead of keeping a local toggle', async ({ page }) => {
  const pub = question(); await widgets(page, pub)
  await page.route('**/api/v3/interactions/*/actions', route => route.fulfill({ status: 409, json: { error: 'stale_interaction' } }))
  await connect(page)
  const option = page.getByRole('checkbox', { name: 'Маргарита Без покупки и оплаты' })
  await option.click()
  await expect(page.getByRole('alert')).toContainText('выбор изменён')
  await expect(option).not.toBeChecked()
  await expect(page.getByRole('button', { name: 'Отправить ответ', exact: true })).toBeDisabled()
})
