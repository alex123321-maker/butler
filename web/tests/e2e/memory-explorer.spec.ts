import { expect, test, type Page } from '@playwright/test'
import { fixture, connect, source } from './mind-fixture'
import { memoryEdges } from '../../features/mind/model/memory-explorer'

const original = {
  ...source('source-original', 'Первое сообщение: поливать монстеру вечером.'),
  title: 'Уход за монстерой',
  origin: { sensitivity: 'private', parent_refs: [] },
}
const derived = {
  ...source('source-derived', 'Заметка о вечернем поливе.'),
  title: 'Вечерний полив',
  source_kind: 'model_candidate',
  origin: { sensitivity: 'private', parent_refs: ['source-original'] },
}
const pending = {
  claim_id: 'claim-pending',
  key: 'plant.watering',
  scope_type: 'user',
  scope_id: 'owner',
  summary: 'Поливать вечером',
  status: 'candidate',
  source_id: derived.source_id,
  source_revision: 1,
}
async function explorerFixture(page: Page) {
  await fixture(page)
  await page.route('**/api/v4/memory/browse*', (r) =>
    r.fulfill({
      json: {
        items: [original, derived],
        mode: 'recent',
        has_more: false,
        diagnostics: [],
      },
    }),
  )
  await page.route('**/api/v4/memory', (r) =>
    r.fulfill({ json: { sources: [original, derived], claims: [pending], claim_list_limit: 2000, source_list_limit: 1000, index_jobs: [], semantic_enabled: false } }),
  )
  await page.route('**/api/v4/memory/source?*', (r) => {
    const id = new URL(r.request().url()).searchParams.get('source_id')
    const s =
      id === derived.source_id
        ? derived
        : id === original.source_id
          ? original
          : source(id || 'missing')
    return r.fulfill({
      json: {
        source: s,
        fragments: [{ chunk_id: 1, title: 'Цитата', text: s.body }],
        revisions: [
          {
            revision: 1,
            body: 'Исторический текст',
            created_at: '2026-10-21T09:00:00Z',
          },
        ],
        revision_limit: 5000,
      },
    })
  })
}
const explorer = (page: Page) =>
  page.getByRole('dialog', { name: 'Память', exact: true })
test('home stays limited to three records while the explorer can load more', async ({ page }) => {
  await explorerFixture(page)
  const records = Array.from({ length: 8 }, (_, i) => source(`record-${i}`))
  await page.route('**/api/v4/memory/browse*', r => {
    const offset = Number(new URL(r.request().url()).searchParams.get('offset') || 0)
    return r.fulfill({ json: { items: records.slice(offset, offset + 6), has_more: offset + 6 < records.length, mode: 'recent', diagnostics: [] } })
  })
  await connect(page)
  await expect(page.locator('.mind-memory-result')).toHaveCount(3)
  await page.getByRole('button', { name: 'Обзор памяти ↗' }).click()
  await expect(explorer(page).locator('.memory-list-row')).toHaveCount(6)
  await explorer(page).getByRole('button', { name: 'Загрузить ещё' }).click()
  await expect(explorer(page).locator('.memory-list-row')).toHaveCount(8)
  await page.keyboard.press('Escape')
  await expect(page.locator('.mind-memory-result')).toHaveCount(3)
  await expect(page.locator('.mind-memory').getByRole('button', { name: 'Ещё', exact: true })).toHaveCount(0)
})
async function open(page: Page) {
  await connect(page)
  await page.getByRole('button', { name: 'Обзор памяти ↗' }).click()
  await expect(explorer(page).locator('.memory-detail h3')).toHaveText(
    original.title,
  )
}

test('graph uses explicit provenance only, excluding hidden records and self references', () => {
  expect(memoryEdges([original, derived, source('unrelated')])).toEqual([
    { from: derived.source_id, to: original.source_id, label: 'Происхождение' },
  ])
  expect(memoryEdges([{ ...original, status: 'revoked' }, derived])).toEqual([])
  expect(memoryEdges([{ ...original, redacted: true }, derived])).toEqual([])
  expect(
    memoryEdges([
      {
        ...original,
        origin: { sensitivity: 'private', parent_refs: [original.source_id] },
      },
    ]),
  ).toEqual([])
})

test('reading, source fragments and history stay in one dialog', async ({
  page,
}) => {
  await explorerFixture(page)
  await open(page)
  const dialog = explorer(page)
  await expect(dialog.getByRole('textbox')).toHaveCount(0)
  await expect(dialog.getByText('Исторический текст')).not.toBeVisible()
  await dialog.getByText('Источник и история', { exact: true }).click()
  await dialog.getByText('Фрагменты источника · 1').click()
  await expect(dialog.locator('blockquote')).toHaveText(original.body)
  await dialog.getByText('История версий · 1').click()
  await dialog.getByText('Версия 1 ·').click()
  await expect(dialog.getByText('Исторический текст')).toBeVisible()
  await expect(page.locator('dialog[open]')).toHaveCount(1)
  await page.keyboard.press('Escape')
  await expect(dialog).not.toBeVisible()
})

test('search preserves the inspected record with a visible context and the graph has no dead end', async ({
  page,
}) => {
  await explorerFixture(page)
  await open(page)
  const dialog = explorer(page)
  await page.keyboard.press('Control+k')
  await expect(dialog.getByRole('searchbox')).toBeFocused()
  await dialog.getByRole('searchbox').fill('монстера')
  await expect(dialog.locator('.memory-list-row')).toHaveCount(1)
  await expect(dialog).toContainText('Найдено: 1')
  await expect(dialog.locator('.memory-outside')).toHaveText(
    'Вне текущего списка',
  )
  await expect(dialog.locator('.memory-detail h3')).toHaveText(original.title)
  await dialog
    .getByRole('button', { name: 'Схема связей', exact: true })
    .click()
  await expect(
    dialog.getByRole('button', { name: 'Найденный источник', exact: true }),
  ).toBeVisible()
  await dialog.getByRole('button', { name: 'Очистить запрос' }).click()
  await expect(
    dialog.getByRole('button', { name: derived.title, exact: true }),
  ).toBeVisible()
  await expect(dialog.getByRole('searchbox')).toHaveValue('')
})

test('recent queries resume a search after reopening and are explicitly clearable', async ({
  page,
}) => {
  await explorerFixture(page)
  await open(page)
  const dialog = explorer(page)
  await dialog.getByRole('searchbox').fill('монстера')
  await expect(dialog).toContainText('Найдено: 1')
  await dialog.getByRole('button', { name: 'Очистить запрос' }).click()
  await expect(
    dialog.getByRole('button', { name: 'монстера', exact: true }),
  ).toBeVisible()
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: 'Обзор памяти ↗' }).click()
  await dialog.getByRole('button', { name: 'монстера', exact: true }).click()
  await expect(dialog.getByRole('searchbox')).toHaveValue('монстера')
  await expect(dialog).toContainText('Найдено: 1')
  await dialog.getByRole('button', { name: 'Очистить запрос' }).click()
  await dialog
    .getByRole('button', { name: 'Очистить недавние запросы' })
    .click()
  await expect(dialog.getByLabel('Недавние запросы')).toHaveCount(0)
})

test('empty search provides a direct return to recent records', async ({
  page,
}) => {
  await explorerFixture(page)
  await page.route('**/api/v4/memory/search', (r) =>
    r.fulfill({ json: { items: [], has_more: false, mode: 'text' } }),
  )
  await open(page)
  const dialog = explorer(page)
  await dialog.getByRole('searchbox').fill('неизвестное')
  await expect(dialog).toContainText('По этим словам ничего не нашлось.')
  await dialog.getByRole('button', { name: 'Показать недавние записи' }).click()
  await expect(dialog.locator('.memory-list-row')).toHaveCount(2)
})

test('keyboard opens a record and back/forward re-read the exact selected source', async ({
  page,
}) => {
  await explorerFixture(page)
  await open(page)
  const dialog = explorer(page)
  await dialog.getByRole('searchbox').focus()
  await page.keyboard.press('ArrowDown')
  await expect(dialog.locator('.memory-list-row').first()).toBeFocused()
  await page.keyboard.press('End')
  await expect(dialog.locator('.memory-list-row').last()).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(dialog.locator('.memory-detail h3')).toHaveText(derived.title)
  await dialog.locator('.memory-related').click()
  await expect(dialog.locator('.memory-detail h3')).toHaveText(original.title)
  const backRequest = page.waitForRequest(
    '**/api/v4/memory/source?source_id=source-derived',
  )
  await dialog
    .getByRole('button', { name: 'Предыдущая просмотренная запись' })
    .click()
  await backRequest
  await expect(dialog.locator('.memory-detail h3')).toHaveText(derived.title)
  await dialog
    .getByRole('button', { name: 'Следующая просмотренная запись' })
    .click()
  await expect(dialog.locator('.memory-detail h3')).toHaveText(original.title)
})

test('dragging moves a node, fit resets it and keyboard selects it', async ({
  page,
}) => {
  await explorerFixture(page)
  await open(page)
  const dialog = explorer(page)
  await dialog
    .getByRole('button', { name: 'Схема связей', exact: true })
    .click()
  const node = dialog.getByRole('button', { name: derived.title, exact: true })
  const before = await node.getAttribute('transform'),
    box = (await node.boundingBox())!
  await page.mouse.move(box.x + 30, box.y + 20)
  await page.mouse.down()
  await page.mouse.move(box.x + 80, box.y + 90, { steps: 8 })
  await page.mouse.up()
  expect(await node.getAttribute('transform')).not.toBe(before)
  await expect(dialog.locator('.memory-detail h3')).toHaveText(original.title)
  await dialog.getByRole('button', { name: 'Вписать' }).click()
  await expect(node).toHaveAttribute('transform', before!)
  await node.focus()
  await page.keyboard.press('Enter')
  await expect(dialog.locator('.memory-detail h3')).toHaveText(derived.title)
  await expect(dialog.locator('.memory-edge > path')).toHaveAttribute(
    'marker-end',
    /url/,
  )
  await page.screenshot({ path: 'test-results/memory-explorer-graph.png' })
})

for (const reject of [false, true])
  test(`${reject ? 'rejection' : 'confirmation'} sends the observed active claim and displays only the server decision`, async ({
    page,
  }) => {
    await explorerFixture(page)
    let decided = false
    const previous = {
      ...pending,
      claim_id: 'previous',
      status: 'confirmed',
      source_id: original.source_id,
    }
    await page.route('**/api/v4/memory', (r) =>
      r.fulfill({
        json: {
          claims: [
            {
              ...pending,
              status: decided
                ? reject
                  ? 'rejected'
                  : 'confirmed'
                : 'candidate',
            },
            previous,
          ],
          claim_list_limit: 2000,
        },
      }),
    )
    await page.route('**/api/v4/memory/decisions', (r) => {
      expect(r.request().postDataJSON()).toEqual({
        claim_id: pending.claim_id,
        expected_active_id: 'previous',
        reject,
      })
      decided = true
      return r.fulfill({
        json: { ...pending, status: reject ? 'rejected' : 'confirmed' },
      })
    })
    await open(page)
    const dialog = explorer(page)
    await dialog.locator('.memory-list-row').last().click()
    await dialog
      .getByRole('button', {
        name: reject ? 'Отклонить факт' : 'Подтвердить факт',
      })
      .click()
    await expect(dialog.getByLabel('Факты из источника')).toContainText(
      reject ? 'Отклонено' : 'Подтверждено вами',
    )
    await expect(
      dialog.getByRole('button', { name: 'Подтвердить факт' }),
    ).toHaveCount(0)
  })

test('conflicting confirmation requires a fresh read before retry', async ({
  page,
}) => {
  await explorerFixture(page)
  let attempts = 0
  await page.route('**/api/v4/memory/decisions', (r) => {
    attempts++
    return r.fulfill({ status: 409, json: { error: 'stale' } })
  })
  await open(page)
  const dialog = explorer(page)
  await dialog.locator('.memory-list-row').last().click()
  await dialog.getByRole('button', { name: 'Подтвердить факт' }).click()
  await expect(dialog).toContainText('Решение не подтверждено.')
  await expect(
    dialog.getByRole('button', { name: 'Подтвердить факт' }),
  ).toBeDisabled()
  expect(attempts).toBe(1)
  const fresh = page.waitForRequest(
    '**/api/v4/memory/source?source_id=source-derived',
  )
  await dialog.getByRole('button', { name: 'Проверить состояние' }).click()
  await fresh
  await expect(
    dialog.getByRole('button', { name: 'Подтвердить факт' }),
  ).toBeEnabled()
  expect(attempts).toBe(1)
})

test('pagination retains results after failure and extends graph on retry', async ({
  page,
}) => {
  await explorerFixture(page)
  let failure = true
  await page.route('**/api/v4/memory/browse*', (r) => {
    const offset = Number(new URL(r.request().url()).searchParams.get('offset'))
    if (offset && failure) return r.fulfill({ status: 503, json: {} })
    return r.fulfill({
      json: {
        items: offset ? [derived] : [original],
        mode: 'recent',
        has_more: !offset,
        diagnostics: [],
      },
    })
  })
  await open(page)
  const dialog = explorer(page)
  await dialog.getByRole('button', { name: 'Загрузить ещё' }).click()
  await expect(dialog).toContainText('Память пока недоступна.')
  await expect(dialog.locator('.memory-list-row')).toHaveCount(1)
  failure = false
  await dialog.getByRole('button', { name: 'Загрузить ещё' }).click()
  await expect(dialog.locator('.memory-list-row')).toHaveCount(2)
  await expect(
    dialog.getByRole('button', { name: 'Загрузить ещё' }),
  ).toHaveCount(0)
  await dialog
    .getByRole('button', { name: 'Схема связей', exact: true })
    .click()
  await expect(dialog.locator('.memory-edge')).toHaveCount(1)
})

test('delayed source cannot replace a newer selection', async ({ page }) => {
  await explorerFixture(page)
  let release: () => void = () => {}
  const wait = new Promise<void>((resolve) => {
    release = resolve
  })
  await page.route(
    '**/api/v4/memory/source?source_id=source-derived',
    async (r) => {
      await wait
      await r.fulfill({
        json: { source: { ...derived, body: 'STALE_DETAIL' }, fragments: [] },
      })
    },
  )
  await open(page)
  const dialog = explorer(page)
  const started = page.waitForRequest(
    '**/api/v4/memory/source?source_id=source-derived',
  )
  await dialog.locator('.memory-list-row').last().click()
  await started
  await dialog.locator('.memory-list-row').first().click()
  await expect(dialog.locator('.memory-detail h3')).toHaveText(original.title)
  const returned = page.waitForResponse(
    '**/api/v4/memory/source?source_id=source-derived',
  )
  release()
  await returned
  await page.evaluate(() => new Promise(requestAnimationFrame))
  await expect(dialog.getByText('STALE_DETAIL')).toHaveCount(0)
})

test('owner change clears recent queries, navigation and delayed private claims', async ({
  page,
}) => {
  await explorerFixture(page)
  let release: () => void = () => {}
  const wait = new Promise<void>((resolve) => {
    release = resolve
  })
  await page.route('**/api/v4/memory', async (r) => {
    await wait
    await r.fulfill({
      json: {
        claims: [
          {
            ...pending,
            source_id: original.source_id,
            summary: 'STALE_PRIVATE_CLAIM',
          },
        ],
        claim_list_limit: 2000,
      },
    })
  })
  await open(page)
  const dialog = explorer(page)
  await dialog.getByRole('searchbox').fill('PRIVATE_QUERY')
  await expect(dialog).toContainText('Найдено: 1')
  await dialog.getByRole('button', { name: 'Очистить запрос' }).click()
  await expect(
    dialog.getByRole('button', { name: 'PRIVATE_QUERY', exact: true }),
  ).toBeVisible()
  await page.keyboard.press('Escape')
  await page
    .getByRole('button', { name: 'Настройки', exact: true })
    .first()
    .click()
  await page
    .getByLabel('Доступ владельца', { exact: true })
    .fill('another-owner')
  release()
  await expect(page.getByText('STALE_PRIVATE_CLAIM')).toHaveCount(0)
  await page.getByRole('button', { name: 'Подключить', exact: true }).click()
  await expect(
    page.getByRole('button', { name: 'Обновить подключение', exact: true }),
  ).toBeVisible()
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Закрыть', exact: true })
    .click()
  await page.getByRole('button', { name: 'Обзор памяти ↗' }).click()
  await expect(dialog.getByText('PRIVATE_QUERY')).toHaveCount(0)
  await expect(
    dialog.getByRole('button', { name: 'Предыдущая просмотренная запись' }),
  ).toBeDisabled()
})

test('copy uses the inspected full text and source management keeps exact identity', async ({
  page,
}) => {
  await explorerFixture(page)
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'clipboard', {
      value: {
        writeText: async (text: string) => {
          document.documentElement.dataset.copied = text
        },
      },
    })
  })
  await open(page)
  const dialog = explorer(page)
  await dialog.getByRole('button', { name: 'Копировать текст' }).click()
  await expect(
    dialog.getByRole('status').filter({ hasText: 'Текст скопирован' }),
  ).toBeVisible()
  expect(
    await page.evaluate(() => document.documentElement.dataset.copied),
  ).toBe(original.body)
  await dialog.getByText('Источник и история', { exact: true }).click()
  await dialog
    .getByRole('button', { name: 'Управлять этой записью ↗' })
    .click()
  await expect(page).toHaveURL(/\/memory\/manage\?source_id=source-original$/)
  await expect(
    page.getByRole('dialog', {
      name: 'Хранилище и политики памяти',
      exact: true,
    }),
  ).toBeVisible()
})

for (const width of [320, 390, 768, 1440])
  test(`single tap reading and viewport bounds at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 820 })
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await explorerFixture(page)
    await connect(page)
    await page.getByRole('button', { name: 'Обзор памяти ↗' }).click()
    const dialog = explorer(page)
    if (width <= 700) {
      await expect(dialog.locator('.memory-detail')).not.toBeVisible()
      await dialog.locator('.memory-list-row').first().click()
      await expect(dialog.locator('.memory-detail h3')).toHaveText(
        original.title,
      )
      await expect(dialog.locator('.memory-body').first()).toBeVisible()
      await expect(page.locator('dialog[open]')).toHaveCount(1)
      await page.screenshot({ path: `test-results/memory-reader-${width}.png` })
      await page.keyboard.press('Escape')
      await expect(dialog).toBeVisible()
      await expect(dialog.locator('.memory-list-row').first()).toBeFocused()
    } else
      await expect(dialog.locator('.memory-detail h3')).toHaveText(
        original.title,
      )
    const bounds = (await dialog.boundingBox())!
    expect(bounds.x).toBeGreaterThanOrEqual(0)
    expect(bounds.y).toBeGreaterThanOrEqual(0)
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(width)
    expect(bounds.y + bounds.height).toBeLessThanOrEqual(820)
    expect(
      await dialog.evaluate(
        (el) =>
          el.scrollHeight <= el.clientHeight &&
          el.scrollWidth <= el.clientWidth,
      ),
    ).toBe(true)
    await page.screenshot({ path: `test-results/memory-explorer-${width}.png` })
  })
