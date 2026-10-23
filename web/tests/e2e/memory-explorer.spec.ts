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
    r.fulfill({ json: { claims: [pending], claim_list_limit: 2000 } }),
  )
  await page.route('**/api/v4/memory/source?*', (r) => {
    const id = new URL(r.request().url()).searchParams.get('source_id')
    const s = id === derived.source_id ? derived : original
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
async function open(page: Page) {
  await connect(page)
  await page.getByRole('button', { name: 'Обзор памяти ↗' }).click()
  await expect(explorer(page).locator('.memory-detail h3')).toHaveText(
    original.title,
  )
}

test('graph uses explicit provenance only, excluding hidden records and self references', () => {
  const sameScope = source('unrelated')
  expect(memoryEdges([original, derived, sameScope])).toEqual([
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

test('list, graph, provenance and history share a selection without editing source text', async ({
  page,
}) => {
  await explorerFixture(page)
  await open(page)
  const dialog = explorer(page)
  await expect(dialog.getByRole('textbox')).toHaveCount(0)
  await dialog.getByRole('button', { name: 'Граф', exact: true }).click()
  await expect(
    dialog.getByRole('button', { name: derived.title, exact: true }),
  ).toBeVisible()
  await dialog.getByRole('button', { name: derived.title, exact: true }).click()
  await expect(dialog.locator('.memory-detail h3')).toHaveText(derived.title)
  await dialog.getByRole('button', { name: 'Записи', exact: true }).click()
  await expect(dialog.locator('.memory-list-row.selected')).toContainText(
    derived.title,
  )
  await dialog.getByRole('button', { name: 'Почему связаны?' }).click()
  await expect(
    page.getByRole('dialog', { name: 'Почему связаны?' }),
  ).toContainText('Сохранённая ссылка происхождения')
  await page.keyboard.press('Escape')
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: 'История', exact: true }).click()
  await page.getByText('Версия 1 ·').click()
  await expect(
    page.getByRole('dialog', { name: 'История записи' }),
  ).toContainText('Исторический текст')
  await page.keyboard.press('Escape')
  await dialog.getByRole('button', { name: 'Открыть источник ↗' }).click()
  await expect(
    page.getByRole('dialog', { name: 'Сохранённый источник' }),
  ).toContainText(derived.body)
  await page.keyboard.press('Escape')
  await expect(dialog).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(dialog).not.toBeVisible()
})

test('search filters both views, preserves selected detail, and can be cleared with shortcut focus', async ({
  page,
}) => {
  await explorerFixture(page)
  await open(page)
  const dialog = explorer(page)
  await page.keyboard.press('Control+k')
  await expect(dialog.getByRole('searchbox')).toBeFocused()
  await dialog.getByRole('searchbox').fill('монстера')
  await expect(dialog.locator('.memory-list-row')).toHaveCount(1)
  await expect(dialog).toContainText('Найдено: 1 · По тексту')
  await expect(dialog.locator('.memory-detail h3')).toHaveText(original.title)
  await dialog.getByRole('button', { name: 'Граф', exact: true }).click()
  await dialog.getByRole('button', { name: 'Весь список' }).click()
  await expect(
    dialog.getByRole('button', { name: 'Найденный источник', exact: true }),
  ).toBeVisible()
  await dialog.getByRole('button', { name: 'Очистить запрос' }).click()
  await expect(
    dialog.getByRole('button', { name: derived.title, exact: true }),
  ).toBeVisible()
  await expect(dialog.getByRole('searchbox')).toHaveValue('')
})

test('dragging moves a node, fit resets it and keyboard selects it', async ({
  page,
}) => {
  await explorerFixture(page)
  await open(page)
  const dialog = explorer(page)
  await dialog.getByRole('button', { name: 'Граф', exact: true }).click()
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
  await page.screenshot({ path: 'test-results/memory-explorer-graph.png' })
})

test('confirmation sends the observed active claim and displays the server decision', async ({
  page,
}) => {
  await explorerFixture(page)
  let confirmed = false
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
          { ...pending, status: confirmed ? 'confirmed' : 'candidate' },
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
      reject: false,
    })
    confirmed = true
    return r.fulfill({ json: { ...pending, status: 'confirmed' } })
  })
  await open(page)
  const dialog = explorer(page)
  await dialog
    .locator('.memory-list-row')
    .filter({ hasText: derived.title })
    .click()
  await dialog.getByRole('button', { name: 'Подтвердить факт' }).click()
  await expect(dialog).toContainText('Подтверждено вами')
  await expect(
    dialog.getByRole('button', { name: 'Подтвердить факт' }),
  ).toHaveCount(0)
})

test('a conflicting confirmation is not displayed as successful or automatically repeated', async ({
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
  await dialog
    .locator('.memory-list-row')
    .filter({ hasText: derived.title })
    .click()
  await dialog.getByRole('button', { name: 'Подтвердить факт' }).click()
  await expect(dialog).toContainText('Решение не подтверждено.')
  await expect(
    dialog.getByRole('button', { name: 'Подтвердить факт' }),
  ).toBeDisabled()
  expect(attempts).toBe(1)
})

test('pagination extends the graph and a failed next page preserves the loaded list for retry', async ({
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
  await dialog.getByRole('button', { name: 'Граф', exact: true }).click()
  await expect(dialog.locator('.memory-edge')).toHaveCount(1)
})

test('a delayed source read cannot overwrite a newer selection inside the explorer', async ({
  page,
}) => {
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
  await dialog
    .locator('.memory-list-row')
    .filter({ hasText: derived.title })
    .click()
  await started
  await dialog
    .locator('.memory-list-row')
    .filter({ hasText: original.title })
    .click()
  await expect(dialog.locator('.memory-detail h3')).toHaveText(original.title)
  release()
  await expect(dialog.getByText('STALE_DETAIL')).toHaveCount(0)
  await expect(dialog.locator('.memory-detail h3')).toHaveText(original.title)
})

test('changing owner clears source, pins and a delayed claim response', async ({
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
  await dialog.getByRole('button', { name: 'Закрепить в просмотре' }).click()
  await expect(dialog.getByRole('button', { name: 'Открепить' })).toBeVisible()
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
  await expect(page.getByText(original.body)).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Открепить' })).toHaveCount(0)
})

for (const width of [320, 390, 768, 1440])
  test(`memory fits the viewport at ${width}px and nested dialogs close independently`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 820 })
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await explorerFixture(page)
    await connect(page)
    await page.getByRole('button', { name: 'Обзор памяти ↗' }).click()
    const dialog = explorer(page)
    if (width <= 700) {
      await dialog.getByRole('button', { name: 'Открыть запись' }).click()
      await expect(
        page.getByRole('dialog', { name: 'Запись памяти', exact: true }),
      ).toContainText(original.body)
      await page.keyboard.press('Escape')
      await expect(dialog).toBeVisible()
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
