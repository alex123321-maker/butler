import { expect, test } from '@playwright/test'

test('owner goals distinguish assumptions and revoke current mandate authority', async ({ page }) => {
  const token = 'fixture-owner-token-000000000000000'
  const goal = { goal_id: 'goal-fixture', revision: 1, kind: 'objective', status: 'active', title: 'Maintain the test service', purpose: 'service_maintenance', constraints: ['Keep work account out of scope'], expires_at: '2027-01-01T00:00:00Z', source: { author: 'fixture-owner', channel: 'web', delivery_id: 'goal-owner' } }
  const assumption = { ...goal, goal_id: 'assumption-fixture', kind: 'assumption', status: 'proposed', title: 'An unconfirmed guess' }
  const mandate = { mandate_id: 'mandate-fixture', revision: 1, goal_id: goal.goal_id, goal_revision: 1, status: 'active', purpose: goal.purpose, tools: ['http.request'], effects: ['read', 'commit'], resource_scopes: [{ origin: 'https://test.example.invalid', path_prefix: '/test-service' }], privacy: 'cloud_allowed', expires_at: goal.expires_at, budget: { model_turns: 20, tokens: 20000, external_calls: 10, children: 2 }, reporting: 'exceptions' }
  let revoked = false
  await page.route('**/api/v3/owner-authority', async route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${token}`)
    await route.fulfill({ json: { goals: [goal, assumption], mandates: [{ ...mandate, status: revoked ? 'revoked' : 'active', revision: revoked ? 2 : 1 }], obligations: [{ task_id: 'task-fixture', goal_id: goal.goal_id, goal_revision: 1, mandate_id: mandate.mandate_id, mandate_revision: 1, initiative_reason: 'Recover test service within agreed procedures', observation_ref: 'observation:fixture', task_status: revoked ? 'paused' : 'active', authority_current: !revoked }], pending_propagation: revoked ? 1 : 0 } })
  })
  await page.route('**/api/v3/owner-mandates', async route => {
    const req = route.request().postDataJSON()
    expect(req.expected_revision).toBe(1)
    expect(req.mandate.status).toBe('revoked')
    expect(route.request().headers()['x-butler-delivery-id']).toBeTruthy()
    revoked = true
    await route.fulfill({ json: { subject_id: mandate.mandate_id, revision: 2, propagation: 'pending' } })
  })
  await page.goto('/goals')
  await expect(page.getByRole('button', { name: 'Load owner authority' })).toBeDisabled()
  await page.getByLabel('Owner control token').fill(token)
  await page.getByRole('button', { name: 'Load owner authority' }).click()
  await expect(page.getByText('assumption · proposed · version 1', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Confirm as new objective' })).toBeVisible()
  await expect(page.getByText('Recover test service within agreed procedures')).toBeVisible()
  await page.getByRole('button', { name: 'Revoke mandate', exact: true }).click()
  await expect(page.getByText('New actions under old authority are already denied.', { exact: false })).toBeVisible()
  await expect(page.getByText('Authority stale or inactive', { exact: false })).toBeVisible()
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain(token)
})
