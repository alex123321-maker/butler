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
