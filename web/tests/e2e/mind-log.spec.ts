import { test, expect } from '@playwright/test'
import type { TranscriptToolCall } from '../../composables/useApi'
import type { RunEvent } from '../../composables/useRunEvents'
import { eventLabels, runLogRows } from '../../features/mind/model/run-log'
import { fixture, connect } from './mind-fixture'

const at = '2026-10-21T09:00:00.450Z'
const event = (
  type: string,
  payload: Record<string, unknown> = {},
  id = type,
): RunEvent => ({
  event_id: id,
  run_id: 'run-mind',
  session_key: 'web:owner',
  event_type: type,
  timestamp: at,
  payload,
})
const call = (id = 'call-1'): TranscriptToolCall => ({
  tool_call_id: id,
  run_id: 'run-mind',
  tool_name: 'memory.search',
  args_json: JSON.stringify({
    query: 'полить монстеру',
    token: 'PRIVATE_LOG_TOKEN',
  }),
  status: 'completed',
  runtime_target: 'memory',
  started_at: '2026-10-21T09:00:00Z',
  finished_at: at,
  result_json: JSON.stringify({
    items: [{ body: 'Полить монстеру вечером' }],
    count: 1,
  }),
  error_json: '',
})
const sse = (events: RunEvent[]) =>
  events
    .map((e) => `event: ${e.event_type}\ndata: ${JSON.stringify(e)}\n\n`)
    .join('') + 'event: stream_end\ndata: {}\n\n'

test('error summary masks the same credentials as expanded event data', () => {
  const rows = runLogRows('run-mind', [], [event('run_error', {
    error_message: 'Upstream rejected Bearer SYNTHETIC_PRIVATE_TOKEN',
  })], [])
  expect(rows[0].text).toContain('Bearer ••••')
  expect(JSON.stringify(rows)).not.toContain('SYNTHETIC_PRIVATE_TOKEN')
})

test('log correlates call IDs, preserves repeated calls and excludes another run', () => {
  const rows = runLogRows(
    'run-mind',
    [],
    [
      event('tool_completed', {
        tool_call_id: 'call-1',
        tool_name: 'memory.search',
      }),
      event('tool_started', {
        tool_call_id: 'call-1',
        tool_name: 'memory.search',
      }),
    ],
    [
      call(),
      {
        ...call('call-2'),
        status: 'failed',
        error_json: '{"message":"Источник недоступен"}',
      },
      { ...call('foreign'), run_id: 'other-run' },
    ],
  )
  expect(rows).toHaveLength(2)
  expect(rows.find((r) => r.id === 'tool-call-1')).toMatchObject({
    type: 'tool_completed',
    pending: false,
    duration: '450 ms',
  })
  expect(rows.find((r) => r.id === 'tool-call-2')).toMatchObject({
    type: 'tool_failed',
    failed: true,
  })
  expect(JSON.stringify(rows)).not.toContain('PRIVATE_LOG_TOKEN')
  expect(JSON.stringify(rows)).toContain('Полить монстеру вечером')
})

test('activity and SSE twins deduplicate despite timestamp precision, while model deltas coalesce', () => {
  const rows = runLogRows(
    'run-mind',
    [
      {
        activity_id: 1,
        run_id: 'run-mind',
        activity_type: 'task_received',
        title: 'Task received',
        summary: '',
        details_json: '{}',
        created_at: '2026-10-21T09:00:00Z',
      },
    ],
    [
      event('memory_loaded'),
      event('assistant_delta', { sequence_no: 1 }, 'delta1'),
      event('assistant_delta', { sequence_no: 2 }, 'delta2'),
    ],
    [],
  )
  expect(rows.map((r) => r.type)).toEqual(['memory_loaded', 'assistant_delta'])
  expect(rows[1].sections[0].text).toContain('2')
})

test('live completion loads durable results immediately; repeated tools show expandable errors', async ({
  page,
}) => {
  await fixture(page, { tasks: true })
  let reads = 0
  await page.route('**/api/v1/runs/run-mind/transcript', (route) =>
    route.fulfill({
      json: {
        messages: [],
        tool_calls:
          ++reads === 1
            ? []
            : [
                call(),
                {
                  ...call('call-2'),
                  status: 'failed',
                  error_json: '{"message":"Источник недоступен"}',
                  result_json: '',
                },
              ],
      },
    }),
  )
  await page.route('**/api/v1/runs/run-mind/events', (route) =>
    route.fulfill({
      contentType: 'text/event-stream',
      body: sse([
        event('tool_started', {
          tool_call_id: 'call-1',
          tool_name: 'memory.search',
        }),
        event('tool_completed', {
          tool_call_id: 'call-1',
          tool_name: 'memory.search',
          status: 'completed',
        }),
        event('assistant_delta', { sequence_no: 1 }),
      ]),
    }),
  )
  await connect(page)
  await expect.poll(() => reads).toBeGreaterThanOrEqual(2)
  await page.getByRole('button', { name: 'Весь журнал' }).click()
  const log = page.getByRole('dialog').getByLabel('Все действия')
  const success = log.locator('[data-event-type="tool_completed"]')
  await expect(success).toHaveCount(1)
  await success.locator('summary').click()
  await expect(success.getByLabel('Аргументы', { exact: true })).toContainText(
    'полить монстеру',
  )
  await expect(
    success.getByLabel('Результат инструмента', { exact: true }),
  ).toContainText('Полить монстеру вечером')
  await expect(
    success.getByLabel('Аргументы', { exact: true }),
  ).not.toContainText('PRIVATE_LOG_TOKEN')
  const failure = log.locator('[data-event-type="tool_failed"]')
  await failure.locator('summary').click()
  await expect(failure.getByLabel('Ошибка', { exact: true })).toContainText(
    'Источник недоступен',
  )
  await expect(log.locator('summary')).toHaveCount(3)
  await expect(
    log.getByText('Инструмент вернул результат', { exact: true }),
  ).toHaveCount(0)
})

test('every event type has a distinct icon without visible category words; full journal retains events', async ({
  page,
}) => {
  await fixture(page, { tasks: true })
  const types = Object.keys(eventLabels)
  const events = types
    .filter((t) => t !== 'tool_failed')
    .map((type, i) => ({
      ...event(
        type,
        type.startsWith('tool_')
          ? { tool_call_id: type, tool_name: 'browser.navigate' }
          : {},
      ),
      timestamp: `2026-10-21T09:00:${String(i).padStart(2, '0')}.000Z`,
    }))
  events.push(
    event(
      'tool_completed',
      { tool_call_id: 'failed-call', tool_name: 'http.fetch', has_error: true },
      'failure',
    ),
  )
  await page.route('**/api/v1/runs/run-mind/events', (route) =>
    route.fulfill({ contentType: 'text/event-stream', body: sse(events) }),
  )
  await connect(page)
  await expect(
    page.getByLabel('Последние действия').locator('details'),
  ).toHaveCount(3)
  await page.getByRole('button', { name: 'Весь журнал' }).click()
  const log = page.getByRole('dialog').getByLabel('Все действия')
  await expect(log.locator('details')).toHaveCount(types.length)
  const paths = await log
    .locator('summary > svg:first-of-type path')
    .evaluateAll((nodes) => nodes.map((n) => n.getAttribute('d')))
  expect(new Set(paths).size).toBe(types.length)
  for (const label of Object.values(eventLabels))
    await expect(log.getByText(label, { exact: true })).toHaveCount(0)
})

test('expanded tool output stays escaped and usable on a narrow screen and clears on owner change', async ({
  page,
}) => {
  await page.setViewportSize({ width: 360, height: 800 })
  await fixture(page, { tasks: true })
  await page.route('**/api/v1/runs/run-mind/transcript', (route) =>
    route.fulfill({
      json: {
        messages: [],
        tool_calls: [
          {
            ...call(),
            result_json: `<img src=x onerror="alert(1)"> ${'long-value-'.repeat(350)}`,
          },
        ],
      },
    }),
  )
  await connect(page)
  await page.getByRole('button', { name: 'Весь журнал' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.locator('[data-event-type="tool_completed"] summary').click()
  await expect(
    dialog.getByLabel('Результат инструмента', { exact: true }),
  ).toContainText('<img src=x')
  await expect(dialog.locator('.mind-log img')).toHaveCount(0)
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(
    true,
  )
  await dialog.getByRole('button', { name: 'Закрыть', exact: true }).click()
  await page.getByLabel('Настройки', { exact: true }).click()
  await page
    .getByLabel('Доступ владельца', { exact: true })
    .fill('another-owner')
  await expect(page.locator('.mind-log')).toHaveCount(0)
})
