import { expect, test } from '@playwright/test'

test('guarded tab choices keep identical origins distinct and send only owner decision', async ({ page }) => {
  const id = 'a'.repeat(48)
  const first = 'b'.repeat(48)
  const second = 'c'.repeat(48)
  const ownerToken = 'fixture-owner-control-token-00000000'
  let selected = false
  let revoked = false
  let decisions = 0
  await page.route('**/api/v2/approvals', route => route.fulfill({ json: { approvals: [{ approval_id: id, run_id: 'waiting-run', tool_name: 'single_tab.bind.guarded', status: selected ? 'approved' : 'pending', summary: 'Select one guarded browser origin' }] } }))
  await page.route(`**/api/v3/tab-choices/${id}`, route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${ownerToken}`)
    return route.fulfill({ json: { approval_id: id, state: selected ? 'selected' : 'pending', expires_at: '2030-01-01T00:00:00Z', candidates: [
      { candidate_token: first, origin: 'https://same.example', ordinal: 1, active_at_request: true, selected: false },
      { candidate_token: second, origin: 'https://same.example', ordinal: 2, active_at_request: false, selected }
    ] } })
  })
  await page.route(`**/api/v3/tab-choices/${id}/select`, async route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${ownerToken}`)
    expect(route.request().postDataJSON()).toEqual({ candidate_token: second })
    decisions++
    selected = true
    await route.fulfill({ json: { single_tab_session_id: 'opaque-selected-session' } })
  })
  await page.route(`**/api/v3/tab-choices/${id}/revoke`, async route => {
    expect(route.request().postDataJSON()).toEqual({})
    revoked = true
    await route.fulfill({ json: { revoked: true } })
  })
  await page.goto('/approvals')
  await expect(page.getByText('Enter the owner token to view tab choices.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Approve', exact: true })).toHaveCount(0)
  await page.getByLabel('Owner control token').fill(ownerToken)
  await expect(page.getByRole('button', { name: 'Tab 1 · https://same.example' })).toBeVisible()
  await page.getByRole('button', { name: 'Tab 2 · https://same.example' }).click()
  await expect(page.getByText('Tab selected. No browser action has been executed.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Tab 2 · https://same.example' })).toBeDisabled()
  expect(decisions).toBe(1)
  await page.getByRole('button', { name: 'Revoke tab binding' }).click()
  await expect(page.getByText('Tab binding revoked. Previously recorded effects are retained.')).toBeVisible()
  expect(revoked).toBe(true)
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain(ownerToken)
})

test('guarded tab read failure and changed owner proof discard late choices', async ({ page }) => {
  const id = 'd'.repeat(48)
  let release: (() => void) | undefined
  const held = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v2/approvals', route => route.fulfill({ json: { approvals: [{ approval_id: id, run_id: 'waiting-run', tool_name: 'single_tab.bind.guarded', status: 'pending' }] } }))
  await page.route(`**/api/v3/tab-choices/${id}`, async route => {
    if (route.request().headers().authorization === 'Bearer old-proof') {
      await held
      await route.fulfill({ json: { approval_id: id, state: 'pending', expires_at: '2030-01-01T00:00:00Z', candidates: [{ candidate_token: 'e'.repeat(48), origin: 'https://late.example', ordinal: 1, active_at_request: false, selected: false }] } })
    } else await route.fulfill({ status: 409, json: { error: 'current tab choice authority required' } })
  })
  await page.goto('/approvals')
  await page.getByLabel('Owner control token').fill('old-proof')
  await expect(page.getByText('Loading current tab choices...')).toBeVisible()
  await page.getByLabel('Owner control token').fill('new-proof')
  await expect(page.getByText('Current tab choices are unavailable. No binding was changed.')).toBeVisible()
  release?.()
  await expect(page.getByRole('button', { name: 'Tab 1 · https://late.example' })).toHaveCount(0)
})

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
