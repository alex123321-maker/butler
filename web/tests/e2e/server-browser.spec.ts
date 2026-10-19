import { expect, test } from '@playwright/test'

test('server browser supports viewing, exclusive control, input and explicit return without storing secrets', async ({ page }) => {
  const token = 'fixture-browser-owner-token-000000000000'
  const id = '0123456789abcdef0123456789abcdef'
  const jpeg = (await page.screenshot({ type: 'jpeg' })).toString('base64')
  let mode = 'agent', epoch = 0, sequence = 0, rejectText = false
  const inputs: Record<string, unknown>[] = []
  await page.route('**/api/v4/server-browser', async route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${token}`)
    const command = route.request().postDataJSON() as Record<string, unknown>
    expect(command.owner_id).toBeUndefined(); expect(command.run_id).toBeUndefined()
    if (command.operation === 'list') return route.fulfill({ json: { ok: true, sessions: [{ browser_session_id: id, run_id: 'run-fixture', task_id: 'task-fixture', mode, epoch }] } })
    expect(command.browser_session_id).toBe(id)
    if (command.operation === 'take') { expect(command.epoch).toBe(epoch); expect(String(command.controller)).toHaveLength(32); mode = 'owner'; epoch++ }
    if (command.operation === 'release') { expect(command.epoch).toBe(epoch); mode = 'agent'; epoch++; return route.fulfill({ json: { ok: true } }) }
    if (!['frame', 'take'].includes(String(command.operation))) {
      expect(mode).toBe('owner'); expect(command.epoch).toBe(epoch); expect(command.frame_id).toBeTruthy(); inputs.push(command)
      if (command.operation === 'text' && rejectText) return route.fulfill({ status: 409, json: { error: 'page changed' } })
    }
    sequence++
    return route.fulfill({ json: { ok: true, frame: { jpeg, frame_id: `frame-${sequence}`, width: 1280, height: 720, mode, epoch } } })
  })
  await page.goto('/browser')
  await expect(page.getByRole('button', { name: 'Load browsers' })).toBeDisabled()
  await page.getByLabel('Owner control token').fill(token)
  await page.getByRole('button', { name: 'Load browsers' }).click()
  await expect(page.getByAltText('Live server browser page')).toBeVisible()
  await expect(page.getByText('Viewing agent actions.')).toBeVisible()
  await page.getByRole('button', { name: 'Take control' }).click()
  await expect(page.getByText('You control this browser. Agent browser actions are paused.')).toBeVisible()
  await page.getByAltText('Live server browser page').click({ position: { x: 100, y: 100 } })
  await expect.poll(() => inputs.filter(input => input.operation === 'click').length).toBe(1)
  await page.getByLabel('Remote text').fill('owner-private-password')
  await page.getByRole('button', { name: 'Send text' }).click()
  await expect.poll(() => inputs.filter(input => input.operation === 'text').length).toBe(1)
  await expect(page.getByLabel('Remote text')).toHaveValue('')
  await page.getByRole('application', { name: 'Remote browser viewport' }).press('Tab')
  await expect.poll(() => inputs.filter(input => input.operation === 'key').length).toBe(1)
  rejectText = true
  await page.getByLabel('Remote text').fill('second-private-input')
  await page.getByRole('button', { name: 'Send text' }).click()
  await expect(page.getByText('Browser unavailable or page changed. Refresh and check the page before another input.')).toBeVisible()
  await page.waitForTimeout(1100)
  expect(inputs.filter(input => input.operation === 'text')).toHaveLength(2)
  await page.getByRole('button', { name: 'Return to agent' }).click()
  await expect(page.getByText('Viewing agent actions.')).toBeVisible()
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toMatch(/fixture-browser-owner-token|owner-private-password|second-private-input/)
  await page.getByLabel('Owner control token').fill('changed-proof')
  await expect(page.getByAltText('Live server browser page')).toHaveCount(0)
})
