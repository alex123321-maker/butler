import { expect, type Page } from '@playwright/test'

export const ownerToken = 'mind-owner-fixture-00000000000000000000'
export const source = (id: string, body = 'Сохранённая заметка') => ({
  source_id: id,
  source_kind: 'owner_statement',
  scope_type: 'user',
  scope_id: 'owner',
  title: id,
  body,
  status: 'active',
  revision: 1,
  access_revision: 1,
  content_hash: 'hash',
  created_at: '2026-10-21T09:00:00Z',
  extracted_at: '2026-10-21T09:00:00Z',
})
export const task = {
  task_id: 'task-mind',
  run_id: 'run-mind',
  session_key: 'web:owner',
  goal: 'Подготовить сводку дня',
  task_status: 'active',
  status: 'active',
  run_state: 'model_running',
  needs_user_action: false,
  waiting_reason: '',
  updated_at: new Date().toISOString(),
  source_channel: 'web',
}
export async function fixture(page: Page, options: { tasks?: boolean } = {}) {
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (route.request().method() === 'OPTIONS')
      return route.fulfill({
        status: 204,
        headers: {
          'Access-Control-Allow-Origin': '*',
          'Access-Control-Allow-Headers': '*',
          'Access-Control-Allow-Methods': '*',
        },
      })
    if (path === '/api/v4/memory/access')
      return route.fulfill({ json: { authenticated: true } })
    if (path === '/api/v4/memory/browse')
      return route.fulfill({
        json: {
          items: [source('Последняя заметка'), source('Другой источник')],
          mode: 'recent',
          has_more: false,
          diagnostics: [],
        },
      })
    if (path === '/api/v4/memory/source')
      return route.fulfill({
        json: {
          source: source(
            new URL(route.request().url()).searchParams.get('source_id')!,
          ),
          fragments: [],
        },
      })
    if (path === '/api/v4/memory/search')
      return route.fulfill({
        json: {
          items: [source('Найденный источник', 'Полить монстеру вечером')],
          mode: 'text',
          has_more: false,
          diagnostics: ['owner_semantic_inference_not_admitted'],
        },
      })
    if (path === '/api/v2/tasks')
      return route.fulfill({
        json: {
          items: options.tasks ? [task] : [],
          total: options.tasks ? 1 : 0,
        },
      })
    if (path === '/api/v2/tasks/task-mind')
      return route.fulfill({
        json: {
          task: { ...task, error_summary: '' },
          durable_task: { goal: task.goal, constraints: [] },
          result: {
            has_result: true,
            verified: false,
            outcome_summary: 'Ответ сохранён',
          },
          final_result: {
            result_id: 'result',
            answer_state: 'final',
            effects: [
              {
                action_id: 'action',
                effect_state: 'effect_unknown',
                evidence_refs: [],
              },
            ],
            limitations: [],
          },
          delivery: { state: 'failed' },
          run_ids: ['run-mind'],
        },
      })
    if (path === '/api/v3/tasks/task-mind')
      return route.fulfill({
        json: {
          task: {
            task_id: task.task_id,
            revision: 7,
            status: 'active',
            goal: task.goal,
          },
        },
      })
    if (path === '/api/v2/tasks/task-mind/activity')
      return route.fulfill({ json: { activity: [] } })
    if (path === '/api/v1/runs/run-mind/transcript')
      return route.fulfill({
        json: {
          messages: [
            {
              message_id: 'public',
              role: 'assistant',
              content: 'Сводка сохранена',
              kind: 'publication',
              published: true,
            },
            {
              message_id: 'private',
              role: 'assistant',
              content: 'INTERNAL_ATTEMPT_SECRET',
              kind: 'attempt_final',
              visibility: 'internal',
            },
          ],
        },
      })
    if (path === '/api/v1/runs/run-mind/events')
      return route.fulfill({
        contentType: 'text/event-stream',
        body:
          'event: memory_loaded\ndata: ' +
          JSON.stringify({
            event_id: 'event1',
            run_id: 'run-mind',
            session_key: 'web:owner',
            event_type: 'memory_loaded',
            timestamp: new Date().toISOString(),
            payload: {},
          }) +
          '\n\nevent: stream_end\ndata: {}\n\n',
      })
    if (path === '/api/v2/approvals')
      return route.fulfill({ json: { approvals: [] } })
    if (path === '/api/v3/initiatives')
      return route.fulfill({
        json: { sources: [], schedules: [], digests: [] },
      })
    if (path === '/api/v1/settings')
      return route.fulfill({ json: { components: [] } })
    return route.fulfill({
      status: 404,
      json: { error: 'unknown fixture route' },
    })
  })
}
export async function connect(page: Page, path = '/', proof = ownerToken) {
  await page.goto(path)
  await page.getByLabel('Доступ владельца', { exact: true }).fill(proof)
  await page.getByRole('button', { name: 'Подключить', exact: true }).click()
  if (path === '/') {
    await expect(
      page.getByRole('button', { name: 'Обновить подключение', exact: true }),
    ).toBeVisible()
    await page
      .getByRole('dialog')
      .getByRole('button', { name: 'Закрыть', exact: true })
      .click()
  } else
    await expect(
      page.getByLabel('Доступ владельца', { exact: true }),
    ).toHaveCount(0)
}
