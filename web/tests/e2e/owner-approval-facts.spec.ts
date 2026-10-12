import { expect, test } from '@playwright/test'

test('approval shows exact server consequences and authority without private details', async ({ page }) => {
  await page.route('**/api/v2/approvals*', route => route.fulfill({ json: { approvals: [{
    approval_id: 'exact-approval', run_id: 'run-owner', status: 'pending', tool_name: 'http.request', risk_level: 'high', requested_via: 'both',
    details_json: JSON.stringify({ private: 'PRIVATE_PROPOSAL_DETAILS' }),
    owner_facts: { summary: 'http.request (commit) on https://example.invalid/account', effect: 'commit', resource: 'https://example.invalid/account', task_id: 'task-owner', task_revision: 4, constraints_revision: 3, goal_id: 'goal-owner', goal_revision: 2, mandate_id: 'mandate-owner', mandate_revision: 5, contract_hash: 'a'.repeat(64), args_hash: 'b'.repeat(64), credential_reference_count: 1 },
  }] } }))
  await page.goto('/approvals')
  await expect(page.getByText('http.request (commit) on https://example.invalid/account', { exact: true })).toBeVisible()
  await expect(page.getByText('Task task-owner version 4 · constraints 3', { exact: true })).toBeVisible()
  await expect(page.getByText('Goal goal-owner version 2 · mandate mandate-owner version 5', { exact: true })).toBeVisible()
  await expect(page.getByText('Credential references 1; values hidden.', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Approve', exact: true })).toBeDisabled()
  await expect(page.getByText('PRIVATE_PROPOSAL_DETAILS')).toHaveCount(0)
})

test('artifact reference opens its exact retained record and fences an older preview', async ({ page }) => {
  const artifact = (id: string) => ({ artifact_id: id, title: id, artifact_type: 'checked_result', run_id: 'run-owner', session_key: 'web:owner', created_at: '2026-10-12T00:00:00Z', updated_at: '2026-10-12T00:00:00Z', content_text: id === 'older-record' ? 'OLDER_BODY' : 'EXACT_RETAINED_EVIDENCE', content_json: '', content_format: 'text', summary: '', source_type: 'owner_result', source_ref: 'result-owner' })
  await page.route('**/api/v2/artifacts*', route => route.fulfill({ json: { artifacts: [artifact('older-record'), artifact('checked-artifact')] } }))
  let release: () => void = () => {}
  const delayed = new Promise<void>(resolve => { release = resolve })
  let started: () => void = () => {}
  const oldStarted = new Promise<void>(resolve => { started = resolve })
  await page.route('**/api/v2/artifacts/*', async route => {
    const id = new URL(route.request().url()).pathname.split('/').at(-1)!
    if (id === 'older-record') { started(); await delayed }
    await route.fulfill({ json: { artifact: artifact(id) } })
  })
  await page.goto('/artifacts?artifact_id=older-record')
  await oldStarted
  await page.locator('.app-dialog__backdrop').click({ position: { x: 5, y: 5 } })
  await page.getByRole('row').filter({ hasText: 'checked-artifact' }).getByRole('button', { name: 'Preview', exact: true }).click()
  await expect(page.getByText('EXACT_RETAINED_EVIDENCE', { exact: true })).toBeVisible()
  const oldResponse = page.waitForResponse('**/api/v2/artifacts/older-record')
  release()
  await oldResponse
  await expect(page.getByText('OLDER_BODY')).toHaveCount(0)
})
