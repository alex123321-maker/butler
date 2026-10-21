import { createHash } from 'node:crypto'
import { test, expect, type Page } from '@playwright/test'
import { fixture, connect, ownerToken } from './mind-fixture'
import type { Publication } from '../../features/mind/model/communications'

const document = Buffer.from('Проверка целостности 🧪\nСохранённый файл\n')
const image = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jT0kAAAAASUVORK5CYII=', 'base64')
const file = (ref: string, name: string, mime: string, data: Buffer) => ({ artifact_ref: ref, name, mime, size: data.length, sha256: createHash('sha256').update(data).digest('hex') })
const report: Publication = {
  publication_ref: 'publication:canonical-report', sequence: 7, session_ref: 'telegram-explicit:77:42',
  created_at: '2026-10-21T17:00:00Z', kind: 'report', available: true,
  payload: {
    purpose: 'result', title: 'Полный отчёт 🧪', summary: '<script>window.syntheticXSS=true</script>',
    blocks: [
      { type: 'heading', text: 'Раздел отчёта' }, { type: 'paragraph', text: 'Сохранённый полный текст 🧪' },
      { type: 'quote', text: 'Цитата' }, { type: 'code', text: '<img src=x onerror=window.syntheticXSS=true>' },
      { type: 'list', items: ['Один', 'Два'] }, { type: 'divider' },
      { type: 'table', rows: [['Компонент', 'Результат'], ['Core', 'Подключён'], ['Broker', 'Проверен']] },
      { type: 'image', artifact_ref: 'artifact:image', alt: 'Проверочное изображение' },
      { type: 'document', artifact_ref: 'artifact:document', alt: 'Проверочный документ' },
    ],
  },
  files: [file('artifact:image', 'proof.png', 'image/png', image), file('artifact:document', 'проверка.txt', 'text/plain', document)],
  parts: [
    { channel: 'web', binding_id: 'web-test', binding_version: 3, index: 0, state: 'available' },
    { channel: 'telegram', binding_id: 'bot-test', binding_version: 2, index: 0, state: 'sent', ack: '14', manifest: { version: 1, method: 'sendRichMessage' } },
    { channel: 'telegram', binding_id: 'bot-test', binding_version: 2, index: 1, state: 'delivery_unknown', manifest: { version: 1, method: 'sendRichMessage' } },
  ],
}
async function communications(page: Page, current: () => Publication[] = () => [report], corrupt = false, tasks = false) {
  await fixture(page, { tasks })
  await page.route('**/api/v3/communications/profile', route => route.fulfill({ json: { ready: true, binding_id: 'web-test', binding_version: 3 } }))
  await page.route('**/api/v3/communications?*', route => route.fulfill({ json: {
    items: current(), messages: [{ message_id: 'canonical-owner-input', session_ref: report.session_ref, content: 'Подготовь отчёт', created_at: '2026-10-21T16:59:00Z' }], cursor: 7, has_more: false,
  } }))
  await page.route('**/api/v3/communications/events?*', route => route.fulfill({ contentType: 'text/event-stream', body: 'id: 7\nevent: refresh\ndata: {"cursor":7}\n\n' }))
  await page.route('**/api/v3/communications/*/media?*', route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${ownerToken}`)
    const ref = new URL(route.request().url()).searchParams.get('artifact_ref')
    return route.fulfill({ body: ref === 'artifact:image' ? image : corrupt ? Buffer.from('changed') : document, contentType: ref === 'artifact:image' ? 'image/png' : 'text/plain' })
  })
}

test('a communication profile never selects the most recently active task as a conversation target', async ({ page }) => {
  await communications(page, () => [report], false, true)
  await connect(page)
  await expect(page.getByRole('heading', { name: 'Разговор с Батлером', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Приостановить поручение', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '1 поручений', exact: true })).toBeVisible()
})

test('drain keeps the saved document readable while new messages and replies are disabled', async ({ page }) => {
  await communications(page)
  await page.route('**/api/v3/communications/profile', route => route.fulfill({ json: { ready: true, binding_id: 'web-test', binding_version: 3, drain_only: true } }))
  await connect(page)
  await expect(page.getByRole('heading', { name: 'Полный отчёт 🧪', exact: true })).toBeVisible()
  await expect(page.getByLabel('Сообщение Батлеру', { exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Ответить на публикацию', exact: true })).toBeDisabled()
  await expect(page.getByRole('link', { name: 'Скачать проверка.txt' })).toBeVisible()
})

test('a blocked communication receipt is shown as blocked rather than accepted work', async ({ page }) => {
  await communications(page)
  await page.route('**/api/v3/communications/messages', route => route.fulfill({ status: 202, json: { input_ref: 'blocked-input', status: 'queued' } }))
  await page.route('**/api/v3/inputs/blocked-input', route => route.fulfill({ json: { input_id: 'blocked-input', status: 'blocked', error_code: 'execution_stopped' } }))
  await connect(page)
  await page.getByLabel('Сообщение Батлеру', { exact: true }).fill('Проверка безопасного отказа')
  await page.getByRole('button', { name: 'Отправить сообщение', exact: true }).click()
  await expect(page.getByText('Запрос заблокирован. Требуется проверка причины.', { exact: true })).toBeVisible()
})

test('canonical report renders all typed blocks safely and downloads exact retained bytes', async ({ page }) => {
  await communications(page)
  await connect(page)
  const article = page.locator('[data-publication="publication:canonical-report"]')
  await expect(article.getByRole('heading', { name: 'Полный отчёт 🧪' })).toBeVisible()
  await expect(article.getByRole('cell', { name: 'Проверен', exact: true })).toBeVisible()
  await expect(article.getByAltText('Проверочное изображение')).toBeVisible()
  await expect(article.getByText('<script>window.syntheticXSS=true</script>', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => 'syntheticXSS' in window)).toBe(false)
  await expect(page.getByText('Подготовь отчёт', { exact: true })).toHaveCount(1)
  const pending = page.waitForEvent('download')
  await article.getByRole('link', { name: 'Скачать проверка.txt' }).click()
  const download = await pending, chunks: Buffer[] = []
  const stream = await download.createReadStream()
  if (!stream) throw new Error('download_missing')
  for await (const chunk of stream) chunks.push(Buffer.from(chunk))
  expect(download.suggestedFilename()).toBe('проверка.txt')
  expect(Buffer.concat(chunks)).toEqual(document)
  await article.getByText('Доставка и части', { exact: true }).click()
  await expect(article.getByText('Веб · часть 1 · доступно', { exact: true })).toBeVisible()
  await expect(article.getByText('Telegram · часть 2 · результат неизвестен', { exact: true })).toBeVisible()
})

test('explicit Web reply pins publication/binding and lost ACK retries the exact request', async ({ page }) => {
  await communications(page)
  const requests: { body: string | null; delivery: string }[] = []
  await page.route('**/api/v3/communications/replies', route => {
    requests.push({ body: route.request().postData(), delivery: route.request().headers()['x-butler-delivery-id'] })
    return requests.length === 1 ? route.abort('connectionclosed') : route.fulfill({ status: 202, json: { input_ref: 'web-reply', session_ref: report.session_ref, status: 'queued' } })
  })
  await page.route('**/api/v3/inputs/web-reply', route => route.fulfill({ json: { input_id: 'web-reply', status: 'completed', session_key: report.session_ref } }))
  await connect(page)
  await page.getByRole('button', { name: 'Ответить на публикацию' }).click()
  await page.getByLabel('Сообщение Батлеру', { exact: true }).fill('Продолжи этот разговор')
  await page.getByRole('button', { name: 'Отправить сообщение', exact: true }).click()
  await page.getByRole('button', { name: 'Повторить тот же запрос', exact: true }).click()
  await expect.poll(() => requests.length).toBe(2)
  expect(requests[1]).toEqual(requests[0])
  expect(JSON.parse(requests[0].body!)).toEqual({ binding_id: 'web-test', binding_version: 3, publication_ref: report.publication_ref, text: 'Продолжи этот разговор' })
})

test('unchanged feed refresh retains validated media instead of refetching it', async ({ page }) => {
  await communications(page)
  let mediaReads = 0
  await page.route('**/api/v3/communications/*/media?*', route => {
    mediaReads++
    const ref = new URL(route.request().url()).searchParams.get('artifact_ref')
    return route.fulfill({ body: ref === 'artifact:image' ? image : document })
  })
  let pulses = 0
  await page.route('**/api/v3/communications/events?*', route => {
    pulses++
    return route.fulfill({ contentType: 'text/event-stream', body: 'id: 7\nevent: refresh\ndata: {}\n\n' })
  })
  await connect(page)
  await expect(page.getByAltText('Проверочное изображение')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Скачать проверка.txt' })).toBeVisible()
  const validatedReads = mediaReads
  await expect.poll(() => pulses).toBeGreaterThanOrEqual(3)
  expect(mediaReads).toBe(validatedReads)
})

test('reconnect rechecks source privacy and removes cached media without duplicate publications', async ({ page }) => {
  let revoked = false, connections = 0
  await communications(page, () => revoked ? [{ ...report, available: false, payload: undefined, files: [], parts: [] }] : [report])
  await page.route('**/api/v3/communications/events?*', route => { connections++; return route.fulfill({ contentType: 'text/event-stream', body: 'id: 7\nevent: refresh\ndata: {"cursor":7}\n\n' }) })
  await connect(page)
  await expect(page.getByAltText('Проверочное изображение')).toBeVisible()
  revoked = true
  await expect(page.getByText('Публикация недоступна: доступ или содержимое изменились.', { exact: true })).toBeVisible({ timeout: 10000 })
  await expect(page.getByAltText('Проверочное изображение')).toHaveCount(0)
  await expect(page.locator('[data-publication]')).toHaveCount(1)
  expect(connections).toBeGreaterThan(1)
})

test('corrupt required attachment is never advertised as downloadable', async ({ page }) => {
  await communications(page, () => [report], true)
  await connect(page)
  await expect(page.getByText('Вложение недоступно или изменилось.', { exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Скачать проверка.txt' })).toHaveCount(0)
})

test('unknown profile readiness does not silently send through the legacy route', async ({ page }) => {
  await fixture(page)
  await page.route('**/api/v3/communications/profile', route => route.fulfill({ status: 503, json: {} }))
  await connect(page)
  await expect(page.getByLabel('Сообщение Батлеру', { exact: true })).toBeDisabled()
  await expect(page.getByText('Режим общения пока не удалось проверить. Обновите подключение.', { exact: true })).toBeVisible()
})

test('full Unicode table remains accessible on a narrow screen', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 900 })
  const wide = structuredClone(report)
  wide.payload!.blocks = [{ type: 'table', rows: [Array.from({ length: 20 }, (_, n) => `Столбец ${n} 🧪`), Array.from({ length: 20 }, (_, n) => `Значение ${n} 🧪`)] }]
  await communications(page, () => [wide])
  await connect(page)
  await expect(page.getByRole('cell', { name: 'Значение 19 🧪', exact: true })).toHaveCount(1)
  expect(await page.evaluate(() => window.document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
})
