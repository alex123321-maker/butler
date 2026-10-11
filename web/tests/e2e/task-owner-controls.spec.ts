import { expect, test } from '@playwright/test'

test('owner amendment pins the current task version and stop preserves uncertainty', async ({ page }) => {
  let revision = 4
  let status = 'waiting_for_user'
  let policy = 'normal'
  const kinds: string[] = []
  await page.route('**/api/v2/tasks/task-control/*', (route) => route.fulfill({ json: { activity: [], artifacts: [], messages: [] } }))
  await page.route('**/api/v2/tasks/task-control', (route) => route.fulfill({ json: {
    task: { task_id: 'task-control', run_id: 'run-old', session_key: 'web:owner', status, run_state: 'awaiting_approval', needs_user_action: true, user_action_channel: 'web' },
    summary_bar: { status, risk_level: 'low', source_channel: 'web', started_at: '2026-03-19T00:00:00Z' },
    source: { channel: 'web', source_message_full: 'Prepare and send' }, waiting_state: { user_action_channel: 'web' },
    result: { has_result: false }, error: { has_error: false }, timeline_preview: [], artifacts: [], run_ids: ['run-old'],
  } }))
  await page.route('**/api/v3/tasks/task-control', (route) => route.fulfill({ json: { task: { revision, status, execution_policy: policy } } }))
  await page.route('**/api/v3/tasks/task-control/control', async (route) => {
    expect(route.request().headers().authorization).toBe('Bearer fixture-owner-token-000000000000000')
    expect(route.request().headers()['x-butler-delivery-id']).toBeTruthy()
    const request = route.request().postDataJSON()
    expect(request.expected_revision).toBe(revision)
    kinds.push(request.kind)
    if (request.kind === 'amend') {
      expect(request.instruction).toBe('Only prepare the draft')
      expect(request.execution_policy).toBe('prepare_only')
      policy = request.execution_policy
      status = 'active'
    } else { expect(request.kind).toBe('stop'); status = 'cancelled' }
    revision++
    await route.fulfill({ json: { task: { revision, status, execution_policy: policy }, input_id: 'control-input', effect_boundary: 'dispatch_may_have_committed_reconciliation_required' } })
  })
  await page.goto('/tasks/task-control')
  await expect(page.getByRole('button', { name: 'Stop task', exact: true })).toBeDisabled()
  await page.getByLabel('Owner control token').fill('fixture-owner-token-000000000000000')
  await page.getByLabel('Amendment', { exact: true }).fill('Only prepare the draft')
  await page.getByLabel('Only prepare; deny external changes').check()
  await page.getByRole('button', { name: 'Apply amendment', exact: true }).click()
  await expect(page.getByText('Task version 5 · active · prepare_only', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Stop task', exact: true }).click()
  await expect(page.getByText('Task version 6 · cancelled · prepare_only', { exact: true })).toBeVisible()
  await expect(page.getByText('Previous dispatch may already have committed. Reconciliation is required.', { exact: false })).toBeVisible()
  expect(kinds).toEqual(['amend', 'stop'])
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain('fixture-owner-token')
})

async function openObservedTask(page: import('@playwright/test').Page) {
  await page.route('**/api/v2/tasks/task-observed/*', (route) => route.fulfill({ json: { activity: [], artifacts: [], messages: [] } }))
  await page.route('**/api/v2/tasks/task-observed', (route) => route.fulfill({ json: {
    task: { task_id: 'task-observed', status: 'running', run_state: 'model_running', needs_user_action: false },
    summary_bar: { status: 'running', risk_level: 'unknown', source_channel: 'web' },
    source: { channel: 'web', source_message_full: 'Observed task' }, waiting_state: {},
    result: { has_result: false }, error: { has_error: false }, timeline_preview: [], artifacts: [], run_ids: [],
  } }))
  await page.route('**/api/v3/tasks/task-observed', (route) => route.fulfill({ json: { task: { revision: 4, status: 'active', execution_policy: 'normal' } } }))
  await page.goto('/tasks/task-observed')
  await page.getByLabel('Owner control token').fill('fixture-owner-token-000000000000000')
}

const overview = {
  task_id: 'task-observed', revision: 4, observed_at: '2026-10-11T00:00:00Z',
  assignment: { goal_id: 'goal-1', mandate_id: 'mandate-1', purpose: 'Prepare evidence', initiative_reason: 'Owner requested comparison' },
  initiative: { status: 'deferred', reason: 'budget', importance: 7, confidence: 8, expected_model_calls: 2 },
  children: [{ task_id: 'child-unverified', status: 'active', delegation_state: 'report_ready' }],
  actions: [{ action_id: 'action-1', tool_name: 'http.request', task_revision: 4, effect: 'commit', effect_state: 'effect_unknown' }],
  auxiliary_work: [
    { job_id: 'job-current', kind: 'extract', status: 'running', attempts: 1, task_revision: 4, current_task_version: true, requested_model: 'pinned-extractor', deadline_elapsed: false },
    { job_id: 'job-obsolete', kind: 'summary', status: 'pending', attempts: 2, task_revision: 3, current_task_version: false, deadline_elapsed: false },
    { job_id: 'job-expired', kind: 'index', status: 'failed', attempts: 1, task_revision: 4, current_task_version: true, deadline_elapsed: true },
  ],
  auxiliary_work_truncated: true,
  budget: { root_task_id: 'task-observed', limits: { model_turns: 5 }, reserved: { model_turns: 3 }, model_attempts: 3, known_input_tokens: 20, known_output_tokens: 4, unknown_input_attempts: 1, unknown_output_attempts: 2 },
}

test('owner observations distinguish child results, effects and incomplete shared usage', async ({ page }) => {
  await page.route('**/api/v3/tasks/task-observed/observations', (route) => {
    expect(route.request().headers().authorization).toBe('Bearer fixture-owner-token-000000000000000')
    return route.fulfill({ json: overview })
  })
  await openObservedTask(page)
  await page.getByRole('button', { name: 'Load task observations' }).click()
  await expect(page.getByText('Task result is unverified', { exact: false })).toBeVisible()
  await expect(page.getByText('Effect unknown; reconcile before repeating', { exact: false })).toBeVisible()
  await expect(page.getByText('Observed token subtotal: input 20, output 4.', { exact: true })).toBeVisible()
  await expect(page.getByText('Usage is incomplete: input missing for 1 attempts; output missing for 2 attempts.', { exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Inspect or revoke owner authority' })).toHaveAttribute('href', '/goals')
  await expect(page.getByRole('link', { name: 'Inspect the initiative source' })).toHaveAttribute('href', '/initiatives')
  await expect(page.getByRole('row', { name: 'Model turns 3 5', exact: true })).toBeVisible()
  await expect(page.getByText('requested model pinned-extractor', { exact: false })).toBeVisible()
  await expect(page.getByText('(obsolete version)', { exact: false })).toBeVisible()
  await expect(page.getByText('deadline elapsed', { exact: false })).toBeVisible()
  await expect(page.getByText('Only the latest 50 background jobs are shown.', { exact: true })).toBeVisible()
  await page.getByLabel('Owner control token').fill('')
  await expect(page.getByRole('heading', { name: 'Recorded task observations' })).toHaveCount(0)
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain('fixture-owner-token')
})

test('changing owner proof fences a late observation response and read failure stays unavailable', async ({ page }) => {
  let release: () => void = () => {}
  const delayed = new Promise<void>((resolve) => { release = resolve })
  let started: () => void = () => {}
  const requestStarted = new Promise<void>((resolve) => { started = resolve })
  let count = 0
  await page.route('**/api/v3/tasks/task-observed/observations', async (route) => {
    count++
    if (count === 1) { started(); await delayed; await route.fulfill({ json: overview }) }
    else await route.fulfill({ status: 503, json: { error: 'unavailable' } })
  })
  await openObservedTask(page)
  await page.getByRole('button', { name: 'Load task observations' }).click()
  await requestStarted
  await page.getByLabel('Owner control token').fill('different-fixture-proof')
  const response = page.waitForResponse('**/api/v3/tasks/task-observed/observations')
  release()
  await response
  await page.getByRole('button', { name: 'Load task observations' }).click()
  await expect(page.getByText('Task observations could not be read. Recorded actions and costs are unavailable.', { exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Recorded task observations' })).toHaveCount(0)
})
