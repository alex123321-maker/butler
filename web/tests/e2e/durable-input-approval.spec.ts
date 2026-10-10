import { expect, test } from '@playwright/test'

test('saved approval requires an owner token and sends its exact identity', async ({ page }) => {
  let decided = false
  await page.route('**/api/v2/approvals', (route) => route.fulfill({ json: { approvals: [{ approval_id: 'approval-stable', run_id: 'run-stable', status: decided ? 'approved' : 'pending', args_json: '{"body":"one"}', summary: 'Submit body one', requested_via: 'web', expires_at: '2030-01-01T00:00:00Z' }] } }))
  await page.route('**/api/v2/approvals/approval-stable/approve', async (route) => {
    expect(route.request().headers().authorization).toBe('Bearer fixture-owner-control-token-00000000')
    expect(route.request().headers()['x-butler-delivery-id']).toBeTruthy()
    decided = true
    await route.fulfill({ json: { changed: true } })
  })
  await page.goto('/approvals')
  await expect(page.getByText('Submit body one')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Approve', exact: true })).toBeDisabled()
  await page.getByLabel('Owner control token').fill('fixture-owner-control-token-00000000')
  await page.getByRole('button', { name: 'Approve', exact: true }).click()
  await expect(page.getByRole('cell', { name: 'approved', exact: true })).toBeVisible()
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain('fixture-owner-control-token')
})

test('receipt observes accepted work independently of a completed task', async ({ page }) => {
  let reads = 0
  await page.route('**/api/v3/inputs/input-stable', async (route) => {
    reads++
    await route.fulfill({ json: { input_id: 'input-stable', session_key: 'web:owner', session_sequence: 2, run_id: 'run-stable', status: reads < 2 ? 'queued' : 'completed', checkpoint: 'finalizing', attempts: 1, max_attempts: 3, expires_at: '2030-01-01T00:00:00Z' } })
  })
  await page.goto('/inputs/input-stable')
  await expect(page.getByTestId('receipt-status')).toHaveText('queued')
  await expect(page.getByTestId('receipt-status')).toHaveText('completed')
  await expect(page.getByText('Task completion is verified separately.', { exact: false })).toBeVisible()
  await expect(page.getByRole('link', { name: 'View conversation attempt' })).toHaveAttribute('href', '/tasks/run-stable')
})
