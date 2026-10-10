import { expect, test, type Page } from '@playwright/test'

const routeTitles: Array<{ path: string; title: string; shot: string }> = [
  { path: '/', title: 'Overview', shot: 'overview.png' },
  { path: '/tasks', title: 'Tasks', shot: 'tasks.png' },
  { path: '/tasks/run-1', title: 'Task run-1', shot: 'task-detail.png' },
  { path: '/approvals', title: 'Approvals', shot: 'approvals.png' },
  { path: '/artifacts', title: 'Artifacts', shot: 'artifacts.png' },
  { path: '/memory', title: 'Memory', shot: 'memory.png' },
  { path: '/activity', title: 'Activity', shot: 'activity.png' },
  { path: '/system', title: 'System', shot: 'system.png' },
]

async function installApiMocks(page: Page): Promise<void> {
  await page.route('**/api/v2/overview', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        attention_items: [],
        active_tasks: [],
        recent_results: [],
        system_summary: {},
        counts: {
          attention_items_count: 0,
          active_tasks_count: 0,
          approvals_pending_count: 0,
          failed_tasks_count: 0,
        },
      }),
    })
  })

  await page.route('**/api/v2/tasks**', async (route) => {
    const requestUrl = new URL(route.request().url())
    const pathname = requestUrl.pathname

    if (pathname === '/api/v2/tasks/run-1/debug') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          run: {
            run_id: 'run-1',
            session_key: 'telegram:chat:1',
            status: 'awaiting_approval',
            current_state: 'awaiting_approval',
            model_provider: 'openai',
            provider_session_ref: '',
            autonomy_mode: 'mode_1',
            metadata_json: '{}',
            error_type: '',
            error_message: '',
            started_at: '2026-03-19T00:00:00Z',
            updated_at: '2026-03-19T00:00:00Z',
            finished_at: null,
          },
          transcript: {
            messages: [
              {
                message_id: 'm1',
                run_id: 'run-1',
                role: 'user',
                content: 'Deploy latest build',
                created_at: '2026-03-19T00:00:00Z',
              },
            ],
            tool_calls: [],
          },
        }),
      })
      return
    }

    if (pathname === '/api/v2/tasks/run-1/activity') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          activity: [
            {
              activity_id: 1,
              run_id: 'run-1',
              session_key: 'telegram:chat:1',
              activity_type: 'approval_requested',
              title: 'Approval requested',
              summary: 'Waiting for operator decision',
              details_json: '{"tool":"http.request"}',
              actor_type: 'system',
              severity: 'warning',
              created_at: '2026-03-19T00:00:00Z',
            },
          ],
        }),
      })
      return
    }

    if (pathname === '/api/v2/tasks/run-1/artifacts') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          artifacts: [
            {
              artifact_id: 'art-1',
              run_id: 'run-1',
              session_key: 'telegram:chat:1',
              artifact_type: 'assistant_final',
              title: 'Assistant final response',
              summary: 'Deployment done',
              content_text: 'Deployment completed successfully',
              content_json: '{"ok":true}',
              content_format: 'text',
              source_type: 'message',
              source_ref: 'run-1',
              created_at: '2026-03-19T00:00:00Z',
              updated_at: '2026-03-19T00:00:00Z',
            },
          ],
        }),
      })
      return
    }

    if (pathname === '/api/v2/tasks/run-1') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          task: {
            task_id: 'run-1',
            run_id: 'run-1',
            session_key: 'telegram:chat:1',
            status: 'waiting_for_reply_in_telegram',
            run_state: 'awaiting_approval',
            current_stage: 'awaiting_approval',
            needs_user_action: true,
            user_action_channel: 'telegram',
            waiting_reason: 'approval_required',
            started_at: '2026-03-19T00:00:00Z',
            updated_at: '2026-03-19T00:00:00Z',
            finished_at: null,
            outcome_summary: '',
            error_summary: '',
            risk_level: 'medium',
            source_channel: 'telegram',
            model_provider: 'openai',
            autonomy_mode: 'mode_1',
          },
          summary_bar: {
            status: 'waiting_for_reply_in_telegram',
            risk_level: 'medium',
            source_channel: 'telegram',
            started_at: '2026-03-19T00:00:00Z',
            updated_at: '2026-03-19T00:00:00Z',
            finished_at: null,
          },
          source: {
            channel: 'telegram',
            session_key: 'telegram:chat:1',
            source_message_preview: 'Deploy latest build',
            source_message_full: 'Deploy latest build with approval',
          },
          waiting_state: {
            needs_user_action: true,
            user_action_channel: 'telegram',
            waiting_reason: 'approval_required',
            note: 'Waiting for Telegram action',
          },
          result: {
            outcome_summary: '',
            has_result: false,
          },
          error: {
            error_type: '',
            error_summary: '',
            has_error: false,
          },
          timeline_preview: [],
          artifacts: [],
        }),
      })
      return
    }

    const sort = requestUrl.searchParams.get('sort')
    if (sort) expect(['updated_at_desc', 'updated_at_asc', 'started_at_desc', 'started_at_asc']).toContain(sort)
    for (const key of ['from', 'to']) {
      const value = requestUrl.searchParams.get(key)
      if (value) expect(value).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/)
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        items: [
          {
            task_id: 'run-1',
            run_id: 'run-1',
            session_key: 'telegram:chat:1',
            status: 'waiting_for_reply_in_telegram',
            run_state: 'awaiting_approval',
            needs_user_action: true,
            waiting_reason: 'approval_required',
            source_channel: 'telegram',
            user_action_channel: 'telegram',
            updated_at: '2026-03-19T00:00:00Z',
          },
        ],
        total: 1,
        limit: 50,
        offset: 0,
      }),
    })
  })

  await page.route('**/api/v1/runs/run-1/transcript', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        run: {
          run_id: 'run-1',
          session_key: 'telegram:chat:1',
          status: 'awaiting_approval',
          current_state: 'awaiting_approval',
          model_provider: 'openai',
          provider_session_ref: '',
          autonomy_mode: 'mode_1',
          metadata_json: '{}',
          error_type: '',
          error_message: '',
          started_at: '2026-03-19T00:00:00Z',
          updated_at: '2026-03-19T00:00:00Z',
          finished_at: null,
        },
        messages: [
          {
            message_id: 'm1',
            run_id: 'run-1',
            role: 'user',
            content: 'Deploy latest build',
            created_at: '2026-03-19T00:00:00Z',
          },
          {
            message_id: 'm2',
            run_id: 'run-1',
            role: 'assistant',
            content: 'Need approval before deploy',
            created_at: '2026-03-19T00:00:01Z',
          },
        ],
        tool_calls: [],
      }),
    })
  })

  await page.route('**/api/v1/sessions', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        sessions: [
          {
            session_key: 'telegram:chat:1',
            user_id: 'owner',
            channel: 'telegram',
            created_at: '2026-03-19T00:00:00Z',
            updated_at: '2026-03-19T00:00:00Z',
          },
        ],
      }),
    })
  })

  await page.route('**/api/v1/memory**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        scope_type: 'session',
        scope_id: 'telegram:chat:1',
        working: {
          memory_type: 'working',
          session_key: 'telegram:chat:1',
          run_id: 'run-1',
          goal: 'Deploy latest build',
          entities_json: '{\"service\":\"api\"}',
          pending_steps_json: '[\"Await approval\"]',
          scratch_json: '{}',
          status: 'active',
          source_type: 'task',
          source_id: 'run-1',
          provenance: '{\"source\":\"task\"}',
          created_at: '2026-03-19T00:00:00Z',
          updated_at: '2026-03-19T00:00:00Z',
        },
        profile: [],
        episodic: [],
        chunks: [],
      }),
    })
  })

  await page.route('**/api/v2/approvals', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        approvals: [
          {
            approval_id: 'approval-1',
            run_id: 'run-1',
            status: 'pending',
            tool_name: 'http.request',
            summary: 'Need approval for HTTP request',
            risk_level: 'medium',
          },
        ],
        total: 1,
      }),
    })
  })

  await page.route('**/api/v2/artifacts*', async (route) => {
    const url = route.request().url()
    if (/\/api\/v2\/artifacts\/[^/?]+$/.test(url)) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          artifact: {
            artifact_id: 'art-1',
            run_id: 'run-1',
            session_key: 'telegram:chat:1',
            artifact_type: 'assistant_final',
            title: 'Assistant final response',
            summary: 'Deployment done',
            content_text: 'Deployment completed successfully',
            content_json: '{"ok":true}',
            content_format: 'text',
            source_type: 'message',
            source_ref: 'run-1',
            created_at: '2026-03-19T00:00:00Z',
            updated_at: '2026-03-19T00:00:00Z',
          },
        }),
      })
      return
    }

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        artifacts: [
          {
            artifact_id: 'art-1',
            run_id: 'run-1',
            session_key: 'telegram:chat:1',
            artifact_type: 'assistant_final',
            title: 'Assistant final response',
            summary: 'Deployment done',
            content_text: 'Deployment completed successfully',
            content_json: '{"ok":true}',
            content_format: 'text',
            source_type: 'message',
            source_ref: 'run-1',
            created_at: '2026-03-19T00:00:00Z',
            updated_at: '2026-03-19T00:00:00Z',
          },
        ],
      }),
    })
  })

  await page.route('**/api/v2/activity*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        activity: [
          {
            activity_id: 1,
            run_id: 'run-1',
            session_key: 'telegram:chat:1',
            activity_type: 'approval_requested',
            title: 'Approval requested',
            summary: 'Awaiting confirmation',
            details_json: '{"tool":"http.request"}',
            actor_type: 'system',
            severity: 'warning',
            created_at: '2026-03-19T00:00:00Z',
          },
        ],
      }),
    })
  })

  await page.route('**/api/v2/system', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        health: {
          status: 'degraded',
          degraded_components: ['doctor'],
        },
        doctor: {
          status: 'degraded',
          checked_at: '2026-03-19T00:00:00Z',
          stale: true,
        },
        providers: [{ name: 'openai', active: true, configured: true }],
        queues: { memory_pipeline: { enabled: true, status: 'running' } },
        single_tab_extension: {
          transport_mode: 'native_only',
          relay_enabled: false,
          extension_auth_configured: true,
          relay_heartbeat_ttl_seconds: 60,
          active_sessions: 0,
          host_disconnected_sessions: 0,
          instances: [],
        },
        pending_approvals: 1,
        recent_failures: [
          {
            run_id: 'run-9',
            status: 'failed',
            error: 'timeout',
            updated_at: '2026-03-19T00:00:00Z',
          },
        ],
        degraded_components: ['doctor'],
        partial_errors: [],
      }),
    })
  })
}

test.describe('Task-centric pages baseline', () => {
  test.use({ timezoneId: 'Asia/Tomsk' })
  test.beforeEach(async ({ page }) => {
    page.on('pageerror', (error) => {
      throw error
    })
    await installApiMocks(page)
  })

  test('smoke: all key pages render headings', async ({ page }) => {
    await page.goto('/')
    await expect(page.getByRole('heading', { name: 'Overview', exact: true })).toBeVisible()

    await page.goto('/tasks')
    await expect(page.getByRole('heading', { name: 'Tasks', exact: true })).toBeVisible()
    await page.getByRole('link', { name: /Task run-1/i }).click()
    await expect(page.getByRole('heading', { name: 'Task run-1', exact: true })).toBeVisible()

    for (const item of routeTitles.filter((route) => route.path !== '/tasks/run-1' && route.path !== '/tasks')) {
      await page.goto(item.path)
      await expect(page.getByRole('heading', { name: item.title, exact: true })).toBeVisible()
    }
  })

  test('navigation: sidebar routes are reachable', async ({ page }) => {
    await page.goto('/')

    await page.getByRole('link', { name: 'Tasks' }).click()
    await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()

    await page.getByRole('link', { name: 'Approvals' }).click()
    await expect(page.getByRole('heading', { name: 'Approvals' })).toBeVisible()

    await page.getByRole('button', { name: 'Show advanced navigation' }).click()
    await page.getByRole('link', { name: 'System' }).click()
    await expect(page.getByRole('heading', { name: 'System' })).toBeVisible()
  })

  test('task filters use API sort values and UTC date bounds', async ({ page }) => {
    await page.goto('/tasks')
    await expect(page.getByText('Waiting for your reply')).toBeVisible()
    await page.getByRole('button', { name: 'Advanced filters' }).click()
    await page.getByRole('combobox', { name: 'Sort', exact: true }).selectOption('started_at_asc')
    await page.getByLabel('From', { exact: true }).fill('2026-03-19T12:30')
    await page.getByLabel('To', { exact: true }).fill('2026-03-20T13:45')
    const request = page.waitForRequest((request) => {
      const url = new URL(request.url())
      return url.pathname === '/api/v2/tasks' && url.searchParams.has('from')
    })
    await page.getByRole('button', { name: 'Apply filters' }).click()
    const url = new URL((await request).url())
    expect(url.searchParams.get('sort')).toBe('started_at_asc')
    expect(url.searchParams.get('from')).toBe('2026-03-19T05:30:00.000Z')
    expect(url.searchParams.get('to')).toBe('2026-03-20T06:45:00.000Z')
    await expect(page.getByText('Waiting for your reply')).toBeVisible()
    await page.getByRole('button', { name: 'Reset', exact: true }).click()
    await page.getByRole('button', { name: 'Advanced filters' }).click()
    await expect(page.getByRole('combobox', { name: 'Sort', exact: true })).toHaveValue('updated_at_desc')
  })

  test('critical states: task waiting status, pending approval, degraded health', async ({ page }) => {
    await page.goto('/tasks')
    await expect(page.getByText('Waiting for your reply')).toBeVisible()
    await expect(page.getByText('Needs your action')).toBeVisible()

    await page.goto('/tasks/run-1')
    await expect(page.getByText('Action is available only through Telegram.')).toBeVisible()
    await expect(page.getByText('Open related memory')).toBeVisible()

    await page.goto('/approvals')
    await expect(page.getByText('Need approval for HTTP request')).toBeVisible()

    await page.goto('/system')
    await expect(page.getByText('degraded', { exact: true }).first()).toBeVisible()
    await expect(page.getByText('Pending approvals: 1')).toBeVisible()
  })

  test('durable task keeps completed attempts separate from verified work', async ({ page }) => {
    await page.route('**/api/v2/tasks/task-durable/*', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ artifacts: [], activity: [] }) })
    })
    await page.route('**/api/v2/tasks/task-durable', async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({
        task: { task_id: 'task-durable', run_id: 'run-2', session_key: 'web:owner', status: 'waiting_for_user', run_state: 'completed', needs_user_action: true, user_action_channel: 'web' },
        summary_bar: { status: 'waiting_for_user', risk_level: 'low', source_channel: 'web', started_at: '2026-03-19T00:00:00Z' },
        source: { channel: 'web', source_message_full: 'Prepare, approve and send' },
        waiting_state: { user_action_channel: 'web', waiting_reason: 'owner_input_required' },
        result: { has_result: true, verified: false, outcome_summary: 'Draft ready, waiting for approval' },
        error: { has_error: false }, timeline_preview: [], artifacts: [], run_ids: ['run-1', 'run-2'],
        durable_task: { goal: 'Prepare, approve and send', constraints: ['Do not send before approval'], working: { plan: ['Prepare', 'Review', 'Send'], pending_actions: ['Send approved draft'], scratch: {} } },
      }) })
    })
    await page.goto('/tasks/task-durable')
    await expect(page.getByText('Waiting for your input', { exact: true })).toBeVisible()
    await expect(page.getByText('Attempts: 2. Latest attempt: completed.')).toBeVisible()
    await expect(page.getByText('Plan: Prepare → Review → Send')).toBeVisible()
    await expect(page.getByText('Constraints: Do not send before approval')).toBeVisible()
    await page.getByRole('button', { name: 'Result', exact: true }).click()
    await expect(page.getByText('Unverified model response', { exact: true })).toBeVisible()
    await expect(page.getByText('Draft ready, waiting for approval', { exact: true })).toBeVisible()
  })

  test('screenshots: key pages visual baseline', async ({ page }) => {
    for (const item of routeTitles) {
      await page.goto(item.path)
      await expect(page).toHaveScreenshot(item.shot, { fullPage: true })
    }
  })
})
