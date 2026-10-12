import { expect, test } from '@playwright/test'

test('workspace discard discovers the copy, keeps exact lost-ACK intent and original effect unknown', async ({ page }) => {
  await page.route('**/api/v2/tasks/recovery-owner*', route => route.fulfill({ json: { task: { task_id: 'recovery-owner', status: 'cancelled' }, summary_bar: { status: 'cancelled' }, source: {}, waiting_state: {}, result: {}, error: {}, timeline_preview: [], artifacts: [], activity: [], run_ids: [] } }))
  await page.route('**/api/v3/tasks/recovery-owner', route => route.fulfill({ json: { task: { revision: 3, status: 'cancelled', execution_policy: 'normal' } } }))
  const status = { workspace_id: 'allocated-unknown-create', task_id: 'recovery-owner', resource_version: 8, workspace_generation: 0, task_revision: 3, constraints_revision: 2, phase: 'quarantined', can_discard: true, original_outcome: 'unknown' }
  let deliveries = 0
  let savedID = ''
  let savedBody = ''
  await page.route('**/api/v3/tasks/recovery-owner/workspace-recovery', route => {
    expect(route.request().headers().authorization).toBe('Bearer fixture-current-owner-proof')
    return route.fulfill({ json: { workspaces: [status], limit: 50, may_have_more: false } })
  })
  await page.route('**/api/workspaces/allocated-unknown-create/discard', async route => {
    deliveries++
    expect(route.request().headers().authorization).toBe('Bearer fixture-current-owner-proof')
    if (deliveries === 1) {
      savedID = route.request().headers()['x-butler-delivery-id']!
      savedBody = route.request().postData()!
      expect(route.request().postDataJSON()).toEqual({ workspace_id: status.workspace_id, resource_version: 8, workspace_generation: 0, task_revision: 3, constraints_revision: 2 })
      await route.abort('connectionclosed')
    } else {
      expect(route.request().headers()['x-butler-delivery-id']).toBe(savedID)
      expect(route.request().postData()).toBe(savedBody)
      await route.fulfill({ json: { ...status, phase: 'requested', can_discard: false, recovery_id: 'saved-recovery-intent' } })
    }
  })
  await page.goto('/tasks/recovery-owner')
  await expect(page.getByRole('button', { name: 'Load workspace recovery' })).toBeDisabled()
  await page.getByLabel('Owner control token').fill('fixture-current-owner-proof')
  await page.getByRole('button', { name: 'Load workspace recovery' }).click()
  await page.getByRole('button', { name: 'Discard this quarantined copy' }).click()
  await expect(page.getByText('Discard outcome is unavailable.', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Retry same discard intent' }).click()
  await expect(page.getByText('Workspace allocated-unknown-create · requested · original outcome unknown', { exact: false })).toBeVisible()
  await expect(page.getByText('Erasure receipt verified.', { exact: false })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Request another discard attempt' })).toBeDisabled()
  await page.getByLabel('Owner control token').fill('another-owner-proof')
  await expect(page.getByText('Workspace allocated-unknown-create · requested', { exact: false })).toHaveCount(0)
  expect(deliveries).toBe(2)
})
