import { expect, test } from '@playwright/test'
import { fixture, connect, ownerToken, source, task } from './mind-fixture'

test('a memory outage leaves the rest of the owner interface usable', async ({
  page,
}) => {
  await fixture(page)
  await page.route('**/api/v4/memory/browse*', (r) =>
    r.fulfill({ status: 503, json: { error: 'catalog unavailable' } }),
  )
  await connect(page)
  await expect(
    page.getByRole('textbox', { name: 'Сообщение Батлеру' }),
  ).toBeEnabled()
  await expect(
    page.getByText('Память пока недоступна. Попробуйте ещё раз.', {
      exact: true,
    }),
  ).toBeVisible()
  await expect(
    page.getByRole('button', { name: 'Браузер', exact: true }),
  ).toBeEnabled()
})

test('a removed source bookmark cannot show retained body or offer correction', async ({
  page,
}) => {
  await fixture(page)
  await page.route('**/api/v4/memory/source?source_id=removed', (r) =>
    r.fulfill({
      json: {
        source: { ...source('removed', 'REMOVED_BODY'), status: 'deleted' },
        fragments: [],
      },
    }),
  )
  await connect(page, '/memory?source_id=removed')
  await expect(
    page.getByText('Этот источник закрыт для редактирования.', {
      exact: false,
    }),
  ).toBeVisible()
  await expect(page.getByLabel('Текст заметки')).toHaveCount(0)
  await expect(page.getByText('REMOVED_BODY')).toHaveCount(0)
})

test('twenty real calendar items create twenty visible vine shoots', async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await fixture(page, { tasks: true })
  await page.route('**/api/v2/tasks?limit=*', (r) =>
    r.fulfill({
      json: {
        items: Array.from({ length: 20 }, (_, i) => ({
          ...task,
          task_id: i === 0 ? 'task-mind' : 'task-' + i,
          goal: 'Поручение ' + (i + 1),
        })),
        total: 20,
      },
    }),
  )
  await connect(page)
  await expect(page.locator('.day-card.is-active .schedule-count')).toHaveText(
    '20 дел',
  )
  await expect(
    page.locator('.garden-sprout[data-progress="1.000"]'),
  ).toHaveCount(20)
  await page.getByRole('button', { name: 'Следующий день' }).click()
  await expect(
    page.locator('.garden-sprout[data-progress="1.000"]'),
  ).toHaveCount(0)
})

test('owner connection, recent memory and server text search keep their real meaning', async ({
  page,
}) => {
  await fixture(page)
  let privateRequests = 0
  page.on('request', (r) => {
    if (r.url().includes('/api/')) privateRequests++
  })
  await page.goto('/')
  await expect(page.getByRole('dialog', { name: 'Настройки' })).toBeVisible()
  expect(privateRequests).toBe(0)
  await page.getByLabel('Доступ владельца', { exact: true }).fill(ownerToken)
  await page.getByRole('button', { name: 'Подключить', exact: true }).click()
  await expect(
    page.getByRole('button', { name: 'Обновить подключение', exact: true }),
  ).toBeVisible()
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Закрыть', exact: true })
    .click()
  await expect(page.locator('.mind-memory-title').first()).toHaveText(
    'Последняя заметка',
  )
  const query = page.waitForRequest('**/api/v4/memory/search')
  await page
    .getByRole('searchbox', { name: 'Поиск в памяти' })
    .fill('растения дома')
  const request = await query
  expect(request.postDataJSON()).toEqual({
    query: 'растения дома',
    limit: 6,
    offset: 0,
  })
  expect(request.headers().authorization).toBe('Bearer ' + ownerToken)
  await expect(
    page.getByText('Найденный источник', { exact: true }),
  ).toBeVisible()
  await expect(page.getByText('По тексту', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Очистить поиск' }).click()
  await expect(page.locator('.mind-memory-title').first()).toHaveText(
    'Последняя заметка',
  )
  await page.reload()
  await expect(
    page.getByLabel('Доступ владельца', { exact: true }),
  ).toHaveValue('')
  await expect(
    page.getByText('Последняя заметка', { exact: true }),
  ).toHaveCount(0)
})

test('changing the owner proof fences a delayed private search and source', async ({
  page,
}) => {
  await fixture(page)
  let release: () => void = () => {}
  const wait = new Promise<void>((resolve) => {
    release = resolve
  })
  await page.route('**/api/v4/memory/search', async (route) => {
    await wait
    await route.fulfill({
      json: {
        items: [source('STALE_PRIVATE_CONTENT')],
        mode: 'text',
        has_more: false,
        diagnostics: [],
      },
    })
  })
  await connect(page)
  const started = page.waitForRequest('**/api/v4/memory/search')
  await page.getByRole('searchbox', { name: 'Поиск в памяти' }).fill('old')
  await started
  await page
    .getByRole('button', { name: 'Настройки', exact: true })
    .first()
    .click()
  await page
    .getByLabel('Доступ владельца', { exact: true })
    .fill('another-owner-proof')
  release()
  await expect(
    page.getByText('Последняя заметка', { exact: true }),
  ).toHaveCount(0)
  await expect(
    page.getByText('STALE_PRIVATE_CONTENT', { exact: true }),
  ).toHaveCount(0)
  await expect(
    page.getByRole('button', { name: 'Подключить', exact: true }),
  ).toBeVisible()
})

test('lost ingress acknowledgement retains the exact durable input without inventing a reply', async ({
  page,
}) => {
  await fixture(page)
  const attempts: { body: unknown; delivery: string }[] = []
  await page.route('**/api/v1/events', async (route) => {
    attempts.push({
      body: route.request().postDataJSON(),
      delivery: route.request().headers()['x-butler-delivery-id'],
    })
    if (attempts.length === 1) return route.abort('failed')
    return route.fulfill({
      status: 202,
      json: {
        accepted: true,
        input_id: 'input-mind',
        receipt_status: 'queued',
        session_key: 'web:owner',
      },
    })
  })
  await connect(page)
  await page
    .getByRole('textbox', { name: 'Сообщение Батлеру' })
    .fill('Напомни полить цветы')
  await page.getByRole('button', { name: 'Отправить сообщение' }).click()
  await expect(
    page.getByRole('button', { name: 'Повторить тот же запрос' }),
  ).toBeVisible()
  await expect(
    page.getByRole('textbox', { name: 'Сообщение Батлеру' }),
  ).toHaveValue('Напомни полить цветы')
  await page.getByRole('button', { name: 'Повторить тот же запрос' }).click()
  await expect(
    page.getByText('Принято в работу', { exact: true }),
  ).toBeVisible()
  expect(attempts).toHaveLength(2)
  expect(attempts[1]).toEqual(attempts[0])
  await expect(page.getByText('Запрос обработан', { exact: true })).toHaveCount(
    0,
  )
  await expect(page.locator('.mind-assistant-message')).toHaveCount(0)
})

test('calendar, observed run log and controls use the actual task; result states remain separate', async ({
  page,
}) => {
  await fixture(page, { tasks: true })
  const controls: { body: unknown; delivery: string }[] = []
  await page.route('**/api/v3/tasks/task-mind/control', async (route) => {
    controls.push({
      body: route.request().postDataJSON(),
      delivery: route.request().headers()['x-butler-delivery-id'],
    })
    if (controls.length === 1) return route.abort('failed')
    return route.fulfill({ json: { ok: true } })
  })
  await connect(page)
  await expect(page.locator('.day-card.is-active .schedule-count')).toHaveText(
    '1 дело',
  )
  await expect(page.locator('.day-card.is-active .task')).toHaveCount(1)
  await expect(
    page.getByText('Сводка сохранена', { exact: true }),
  ).toBeVisible()
  await expect(page.getByText('INTERNAL_ATTEMPT_SECRET')).toHaveCount(0)
  await expect(
    page.locator('.mind-log [data-event-type="memory_loaded"] summary'),
  ).toBeVisible()
  await page.getByRole('button', { name: 'Приостановить поручение' }).click()
  await page
    .getByRole('button', { name: 'Повторить действие', exact: true })
    .click()
  await expect.poll(() => controls.length).toBe(2)
  expect(controls[1]).toEqual(controls[0])
  expect(controls[0].body).toEqual({ kind: 'pause', expected_revision: 7 })
  await page
    .getByRole('heading', { name: 'Подготовить сводку дня' })
    .getByRole('button')
    .click()
  const dialog = page.getByRole('dialog')
  await expect(
    dialog.getByText('Не подтверждена', { exact: true }),
  ).toBeVisible()
  await expect(
    dialog.getByText('Ошибка доставки', { exact: true }),
  ).toBeVisible()
  await expect(
    dialog.getByText('Эффект неизвестен — автоматический повтор недопустим', {
      exact: true,
    }),
  ).toBeVisible()
})

for (const width of [320, 390, 768, 1440])
  test(`continuous layout stays usable at ${width}px with reduced motion`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 980 })
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await fixture(page, { tasks: true })
    await connect(page)
    await expect(
      page.getByRole('searchbox', { name: 'Поиск в памяти' }),
    ).toBeVisible()
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true)
    await page.getByRole('button', { name: 'Следующий день' }).click()
    await expect(
      page.locator('.day-card.is-active .schedule-count'),
    ).toHaveText('0 дел')
    await page.getByRole('button', { name: 'Сегодня ↗', exact: true }).click()
    await expect(
      page.locator('.day-card.is-active .schedule-count'),
    ).toHaveText('1 дело')
    expect(await page.locator('.garden-world').count()).toBe(1)
  })

test('uncertain note saving retries the same source identity, including after closing its panel', async ({
  page,
}) => {
  await fixture(page)
  const attempts: { body: unknown; delivery: string }[] = []
  await page.route('**/api/v4/memory/notes', (r) => {
    attempts.push({
      body: r.request().postDataJSON(),
      delivery: r.request().headers()['x-butler-delivery-id'],
    })
    return attempts.length === 1
      ? r.abort('failed')
      : r.fulfill({ json: source('Saved note') })
  })
  await connect(page)
  await page.getByRole('button', { name: 'Добавить заметку' }).click()
  await page.getByLabel('Название заметки').fill('Вечером')
  await page.getByLabel('Текст заметки').fill('Полить монстеру')
  await page.getByRole('button', { name: 'Сохранить', exact: true }).click()
  await expect(
    page.getByRole('button', { name: 'Повторить сохранение' }),
  ).toBeVisible()
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Закрыть', exact: true })
    .click()
  await page.getByRole('button', { name: 'Добавить заметку' }).click()
  await expect(page.getByLabel('Текст заметки')).toHaveValue('Полить монстеру')
  await page.getByRole('button', { name: 'Повторить сохранение' }).click()
  await expect(page.getByRole('dialog')).not.toBeVisible()
  expect(attempts).toHaveLength(2)
  expect(attempts[1]).toEqual(attempts[0])
})

test('a new memory selection cannot receive the old source body', async ({
  page,
}) => {
  await fixture(page)
  let release: () => void = () => {}
  const wait = new Promise<void>((r) => {
    release = r
  })
  await page.route('**/api/v4/memory/source?source_id=*', async (r) => {
    const id = new URL(r.request().url()).searchParams.get('source_id')!
    if (id === 'Последняя заметка') await wait
    return r.fulfill({
      json: {
        source: source(
          id,
          id === 'Последняя заметка'
            ? 'OLD_SOURCE_BODY'
            : 'CURRENT_SOURCE_BODY',
        ),
        fragments: [],
      },
    })
  })
  await connect(page)
  const pending = page.waitForRequest((r) =>
    r.url().includes('/api/v4/memory/source'),
  )
  await page
    .getByRole('button')
    .filter({ has: page.getByText('Последняя заметка', { exact: true }) })
    .click()
  await pending
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Закрыть', exact: true })
    .click()
  await page
    .getByRole('button')
    .filter({ has: page.getByText('Другой источник', { exact: true }) })
    .click()
  await expect(page.getByLabel('Текст заметки')).toHaveValue(
    'CURRENT_SOURCE_BODY',
  )
  release()
  await expect(page.getByLabel('Текст заметки')).toHaveValue(
    'CURRENT_SOURCE_BODY',
  )
  await expect(page.getByText('OLD_SOURCE_BODY')).toHaveCount(0)
})
