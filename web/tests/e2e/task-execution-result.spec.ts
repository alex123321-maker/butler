import { expect, test } from '@playwright/test'

const cases: Array<{ title: string; effect: string; delivery: string; answer: string; action: string; notice: string; redacted?: boolean }> = [
  { title: 'confirmed action with failed notification', effect: 'effect_confirmed', delivery: 'failed', answer: 'final', action: 'Effect confirmed', notice: 'Notification failed. The saved response remains available.' },
  { title: 'unknown action and notification', effect: 'effect_unknown', delivery: 'delivery_unknown', answer: 'final', action: 'Effect unknown; do not repeat automatically', notice: 'Notification delivery is unknown. It will not be sent again automatically.' },
  { title: 'partial response available in Web', effect: 'effect_absent', delivery: 'result_ready', answer: 'partial', action: 'Effect confirmed absent', notice: 'Response is stored and available in Web. Reading it is not tracked.' },
  { title: 'prepared action without recorded evidence', effect: 'prepared', delivery: 'pending', answer: 'final', action: 'Prepared; effect unverified', notice: 'Notification is waiting for delivery. The saved response remains available.' },
  { title: 'removed response with failed notification', effect: 'effect_confirmed', delivery: 'failed', answer: 'final', action: 'Action evidence status is unavailable.', notice: 'Notification failed.', redacted: true },
]

for (const scenario of cases) {
  test(`task preserves separate result states: ${scenario.title}`, async ({ page }) => {
    await page.route('**/api/v2/tasks/task-state/*', (route) => route.fulfill({ json: { activity: [], artifacts: [], messages: [] } }))
    await page.route('**/api/v3/tasks/task-state', (route) => route.fulfill({ json: { task: { revision: 3, status: 'active', execution_policy: 'normal' } } }))
    await page.route('**/api/v2/tasks/task-state', (route) => route.fulfill({ json: {
      task: { task_id: 'task-state', run_id: 'attempt-new', session_key: 'web:owner', status: 'active', run_state: 'completed' },
      summary_bar: { status: 'active', risk_level: 'low', source_channel: 'web' },
      source: { channel: 'web', source_message_full: 'Prepare a checked correction' }, waiting_state: {},
      result: { has_result: true, verified: false, outcome_summary: 'A response was saved' }, error: {}, timeline_preview: [], artifacts: [],
      run_ids: ['attempt-old', 'attempt-new'],
      durable_task: { goal: 'Prepare a checked correction', constraints: ['Keep the original unapplied'], completion_criteria: ['Independent tests pass'], working: { plan: ['Prepare', 'Check'], pending_actions: ['Owner verification'], scratch: {} } },
      final_result: { result_id: 'result:attempt-new', answer_state: scenario.answer, privacy_redacted: scenario.redacted || false, effects: [{ action_id: 'action-1', effect_state: scenario.effect, evidence_refs: scenario.effect === 'prepared' ? null : ['evidence:1'] }], limitations: ['task_completion_not_verified'] },
      delivery: { channel: 'web', state: scenario.delivery },
    } }))
    await page.goto('/tasks/task-state')
    await expect(page.getByText('Independent tests pass', { exact: true })).toBeVisible()
    await expect(page.getByRole('link', { name: 'attempt-old', exact: true })).toHaveAttribute('href', '/runs/attempt-old')
    await expect(page.getByRole('link', { name: 'attempt-new', exact: true })).toHaveAttribute('href', '/runs/attempt-new')
    await page.getByRole('button', { name: 'Result', exact: true }).click()
    await expect(page.getByText('Unverified model response', { exact: true })).toBeVisible()
    await expect(page.getByText(scenario.action, { exact: true })).toBeVisible()
    await expect(page.getByText(scenario.notice, { exact: true })).toBeVisible()
    if (scenario.answer === 'partial') await expect(page.getByText('A partial response was saved. Its output is incomplete.', { exact: true })).toBeVisible()
    if (scenario.redacted) {
      await expect(page.getByText('Response content was removed under the source retention policy.', { exact: true })).toBeVisible()
      await expect(page.getByText('Evidence references', { exact: true })).toHaveCount(0)
      await expect(page.getByText('Notification failed. The saved response remains available.', { exact: true })).toHaveCount(0)
    } else {
      await page.getByText('Evidence references', { exact: true }).click()
      if (scenario.effect === 'prepared') await expect(page.getByText('No evidence references were saved.', { exact: true })).toBeVisible()
      else await expect(page.getByText('evidence:1', { exact: true })).toBeVisible()
    }
    await expect(page.getByText('Verified user result', { exact: true })).toHaveCount(0)
  })
}

test('unavailable evidence and delivery do not become successful states', async ({ page }) => {
  await page.route('**/api/v2/tasks/task-unavailable/*', (route) => route.fulfill({ json: { activity: [], artifacts: [], messages: [] } }))
  await page.route('**/api/v3/tasks/task-unavailable', (route) => route.fulfill({ json: { task: { revision: 1, status: 'active', execution_policy: 'normal' } } }))
  await page.route('**/api/v2/tasks/task-unavailable', (route) => route.fulfill({ json: {
    task: { task_id: 'task-unavailable', status: 'active' }, summary_bar: { status: 'active' }, source: {}, waiting_state: {}, result: { has_result: false, verified: false }, error: {}, run_ids: [],
    final_result: { state: 'result_read_unavailable' }, delivery: { state: 'delivery_read_unavailable' },
  } }))
  await page.goto('/tasks/task-unavailable')
  await page.getByRole('button', { name: 'Result', exact: true }).click()
  await expect(page.getByText('Saved response status is unavailable.', { exact: true })).toBeVisible()
  await expect(page.getByText('Action evidence status is unavailable.', { exact: true })).toBeVisible()
  await expect(page.getByText('Notification delivery status is unavailable', { exact: true })).toBeVisible()
  await expect(page.getByText('Unverified model response', { exact: true })).toHaveCount(0)
  await expect(page.getByText('Effect confirmed', { exact: true })).toHaveCount(0)
})
