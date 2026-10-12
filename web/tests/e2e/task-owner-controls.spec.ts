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

test('lost control acknowledgement retries the exact durable delivery without a second amendment', async ({ page }) => {
  await openObservedTask(page)
  let firstDelivery = ''
  let firstBody = ''
  let requests = 0
  let applied = 0
  await page.route('**/api/v3/tasks/task-observed/control', async (route) => {
    requests++
    const delivery = route.request().headers()['x-butler-delivery-id']!
    const body = route.request().postData()!
    if (requests === 1) {
      firstDelivery = delivery
      firstBody = body
      applied++
      await route.abort('connectionclosed')
    } else {
      expect(delivery).toBe(firstDelivery)
      expect(body).toBe(firstBody)
      expect(route.request().postDataJSON().expected_revision).toBe(4)
      await route.fulfill({ json: { task: { revision: 5, status: 'active', execution_policy: 'normal' }, input_id: 'saved-control-receipt', effect_boundary: 'previous_attempt_revoked' } })
    }
  })
  await page.getByLabel('Amendment', { exact: true }).fill('Prepare the checked correction')
  await page.getByRole('button', { name: 'Apply amendment', exact: true }).click()
  await expect(page.getByText('Control outcome is unavailable.', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Apply amendment', exact: true })).toBeDisabled()
  // Editing the draft does not alter the saved request being recovered.
  await page.getByLabel('Amendment', { exact: true }).fill('A different later intention')
  await page.getByRole('button', { name: 'Retry same control', exact: true }).click()
  await expect(page.getByText('Task version 5 · active · normal', { exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'View control receipt' })).toHaveAttribute('href', '/inputs/saved-control-receipt')
  expect(requests).toBe(2)
  expect(applied).toBe(1)
})

test('branch stop uses its own observed revision and keeps the parent task current', async ({ page }) => {
  await page.route('**/api/v3/tasks/task-observed/observations', route => route.fulfill({ json: { ...overview, children: [{ task_id: 'child-unverified', revision: 2, status: 'active', delegation_state: 'report_ready' }] } }))
  await openObservedTask(page)
  await page.route('**/api/v3/tasks/child-unverified/control', async route => {
    expect(route.request().postDataJSON()).toEqual({ kind: 'stop', expected_revision: 2, instruction: '' })
    expect(route.request().headers().authorization).toBe('Bearer fixture-owner-token-000000000000000')
    await route.fulfill({ json: { task: { revision: 3, status: 'cancelled', execution_policy: 'normal' }, input_id: 'branch-control', effect_boundary: 'previous_attempt_revoked' } })
  })
  await page.getByRole('button', { name: 'Load task observations' }).click()
  await page.getByRole('button', { name: 'Stop branch child-unverified', exact: true }).click()
  await expect(page.getByText('Task version 4 · active · normal', { exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'View control receipt' })).toHaveAttribute('href', '/inputs/branch-control')
})

test('owner sees pinned sources, saved artifacts, stale verification and separate delivery', async ({ page }) => {
  const hash = 'a'.repeat(64)
  await page.route('**/api/v3/tasks/task-observed/observations', route => route.fulfill({ json: {
    ...overview, status: 'active',
    assignment: { ...overview.assignment, goal_revision: 2, mandate_revision: 3, observation_ref: 'observation:saved-check' },
    authority: { available: true, goal_revision: 4, goal_status: 'active', mandate_revision: 3, mandate_status: 'revoked', versions_current: false },
    attempts: [{ run_id: 'run-completed', task_revision: 3, state: 'completed', answer_state: 'final', delivery_state: 'delivery_unknown', artifact_id: 'final:run-completed', privacy_redacted: false }],
    results: [{ result_id: 'owner-result', task_revision: 3, current_revision: false, verdict: 'verified', evidence_refs: [{ kind: 'source', id: 'source-original' }, { kind: 'artifact', id: 'checked-artifact' }], omitted_reference_count: 1 }],
    actions: [{ ...overview.actions[0], contract_hash: hash, source_refs: [{ source_id: 'source-original', revision: 2, access_revision: 5, content_hash: hash }] }],
  } }))
  await openObservedTask(page)
  await page.getByRole('button', { name: 'Load task observations' }).click()
  await expect(page.getByText('Stored authority is stale or inactive.', { exact: false })).toBeVisible()
  await expect(page.getByText('Observation reference observation:saved-check', { exact: true })).toBeVisible()
  await expect(page.getByText('notification delivery_unknown', { exact: false })).toBeVisible()
  await expect(page.getByText('Result owner-result · verified · task version 3 (obsolete version)', { exact: false })).toBeVisible()
  await expect(page.getByRole('link', { name: 'artifact checked-artifact' })).toHaveAttribute('href', '/artifacts?artifact_id=checked-artifact')
  await page.getByText('Recorded evidence and contract', { exact: true }).click()
  await expect(page.getByRole('link', { name: 'Source source-original', exact: true })).toHaveAttribute('href', '/memory?source_id=source-original')
  await expect(page.getByText('pinned revision 2', { exact: false })).toBeVisible()
})

test('changed owner proof fences a late control receipt', async ({ page }) => {
  await openObservedTask(page)
  let release: () => void = () => {}
  const delayed = new Promise<void>(resolve => { release = resolve })
  let started: () => void = () => {}
  const requestStarted = new Promise<void>(resolve => { started = resolve })
  await page.route('**/api/v3/tasks/task-observed/control', async route => {
    started()
    await delayed
    await route.fulfill({ json: { task: { revision: 5, status: 'paused', execution_policy: 'normal' }, input_id: 'old-proof-receipt', effect_boundary: 'previous_attempt_revoked' } })
  })
  await page.getByRole('button', { name: 'Pause task', exact: true }).click()
  await requestStarted
  await page.getByLabel('Owner control token').fill('another-owner-proof')
  const response = page.waitForResponse('**/api/v3/tasks/task-observed/control')
  release()
  await response
  await expect(page.getByRole('link', { name: 'View control receipt' })).toHaveCount(0)
  await expect(page.getByText('Task version 5 · paused · normal', { exact: true })).toHaveCount(0)
})
