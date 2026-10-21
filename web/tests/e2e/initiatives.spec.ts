import { openMindPanel } from './mind-panel'
import { expect, test } from '@playwright/test'

test('initiative controls preserve quiet decisions, source revisions and current owner proof', async ({ page }) => {
  const token = 'fixture-initiative-owner-token'
  const revision = 'a'.repeat(64)
  const goal = { goal_id: 'goal-fixture', revision: 2, kind: 'objective', status: 'active', title: 'Maintain the fixture', purpose: 'project_maintenance', constraints: [], expires_at: '2027-01-01T00:00:00Z' }
  const mandate = { mandate_id: 'mandate-fixture', revision: 3, goal_id: goal.goal_id, goal_revision: goal.revision, status: 'active', purpose: goal.purpose, allow_initiative: true }
  const source = { source_id: 'source-fixture', goal_id: goal.goal_id, mandate_id: mandate.mandate_id, session_key: 'web:fixture-owner', project_ref: 'fixture-project', project_revision: revision, observer_kind: 'task_project', purpose: goal.purpose, time_zone: 'Asia/Tomsk', task_goal: 'Prepare a checked correction', execution_policy: 'prepare_only', completion_criteria: ['Patch checks pass'], active_start_hour: 9, active_end_hour: 20, failure_threshold: 3, confidence_threshold: 90, minimum_importance: 60, cooldown_seconds: 3600, enabled: true, revision: 4 }
  const candidate = { initiative_id: 'initiative-fixture', source_id: source.source_id, status: 'deferred', reason: 'owner_day_budget_exhausted', occurrences: 3, confidence: 100, importance: 70, expected_model_calls: 2, attention_cost: 1, not_before: '2026-10-12T00:00:00Z' }
  const sources = [source]
  let created = false
  const manifestHash = 'b'.repeat(64)
  await page.route('**/api/v4/extensions', route => route.fulfill({ json: { items: [{ state: { manifest_hash: manifestHash, reviewed: true }, manifest: { id: 'project', version: '1.0.0', operations: [{ name: 'extension.project.inspect', method: 'GET', effect: 'read' }] } }] } }))
  await page.route('**/api/v3/owner-authority', async route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${token}`)
    await route.fulfill({ json: { goals: [goal], mandates: [mandate], obligations: [], pending_propagation: 0 } })
  })
  await page.route('**/api/v3/initiatives', async route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${token}`)
    await route.fulfill({ json: { sources, schedules: [], initiatives: [candidate], events: [{ source_id: source.source_id, event_id: 'event-fixture', verdict: 'empty_check', observed_at: '2026-10-11T00:00:00Z' }, { source_id: source.source_id, event_id: 'event-own', verdict: 'own_change', observed_at: '2026-10-11T00:01:00Z' }], digests: [], attention_used_today: 2, attention_daily_limit: 20 } })
  })
  await page.route('**/api/v3/initiatives/sources', async route => {
    const request = route.request().postDataJSON()
    expect(route.request().headers().authorization).toBe(`Bearer ${token}`)
    expect(route.request().headers()['x-butler-delivery-id']).toBeTruthy()
    if (request.expected_revision === 0) {
      expect(request.source.goal_id).toBe(goal.goal_id)
      expect(request.source.mandate_id).toBe(mandate.mandate_id)
      expect(request.source.project_revision).toBe(revision)
      expect(request.source.observer_kind).toBe('broker_project')
      expect(request.source.check_manifest_hash).toBe(manifestHash)
      expect(request.source.check_tool_name).toBe('extension.project.inspect')
      expect(request.source.check_resource).toBe('https://example.invalid/project')
      expect(request.source.enabled).toBe(false)
      expect(request.source.execution_policy).toBe('prepare_only')
      sources.push({ ...request.source, revision: 1 })
      created = true
    } else {
      expect(request.expected_revision).toBe(source.revision)
      expect(request.source.enabled).toBe(false)
      source.enabled = false
      source.revision++
    }
    await route.fulfill({ json: { kind: 'source', subject_id: request.source.source_id, revision: request.expected_revision + 1 } })
  })
  await page.route('**/api/v3/initiatives/schedules', async route => {
    const request = route.request().postDataJSON()
    expect(request.expected_revision).toBe(0)
    expect(request.schedule.source_id).toBe(source.source_id)
    expect(request.schedule.time_zone).toBe(source.time_zone)
    expect(request.schedule.interval_seconds).toBe(3600)
    expect(request.schedule.missed_policy).toBe('coalesce')
    await route.fulfill({ json: { kind: 'schedule', subject_id: request.schedule.schedule_id, revision: 1 } })
  })
  await openMindPanel(page, '/initiatives')
  await expect(page.getByRole('button', { name: 'Load initiatives' })).toBeDisabled()
  await page.getByLabel('Owner control token').fill(token)
  await page.getByRole('button', { name: 'Load initiatives' }).click()
  await expect(page.getByText('Deferred · Daily execution budget exhausted')).toBeVisible()
  await expect(page.getByText('No grouped notifications. Empty checks and proposals remain quiet.')).toBeVisible()
  await expect(page.getByText('Change from Butler work', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'Pause checks', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Enable checks', exact: true })).toBeVisible()
  await page.getByLabel('Active mandate').selectOption(mandate.mandate_id)
  await page.getByLabel('Observation', { exact: true }).selectOption('broker_project')
  await page.getByLabel('Reviewed check', { exact: true }).selectOption(`${manifestHash}:extension.project.inspect`)
  await page.getByLabel('Exact check URL').fill('https://example.invalid/project')
  await page.getByLabel('Project reference', { exact: true }).fill('second-fixture')
  await page.getByLabel('Project revision', { exact: true }).fill(revision)
  await page.getByLabel('Conversation reference').fill(source.session_key)
  await page.getByLabel('Task to prepare').fill('Prepare a checked correction')
  await page.getByLabel('Acceptance criteria').fill('Patch checks pass')
  await page.getByRole('button', { name: 'Save paused checks' }).click()
  await expect.poll(() => created).toBe(true)
  await page.getByLabel('Project checks', { exact: true }).selectOption(source.source_id)
  await page.getByRole('button', { name: 'Save schedule', exact: true }).click()
  await expect(page.getByText('Configuration saved.', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain(token)
  await page.getByLabel('Owner control token').fill('different-owner-proof')
  await expect(page.getByText('Observed work', { exact: true })).toHaveCount(0)
})

test('a late initiative response does not restore data after owner proof changes', async ({ page }) => {
  let release: () => void = () => undefined
  const received = new Promise<void>(resolve => { release = resolve })
  let requested = false
  await page.route('**/api/v4/extensions', route => route.fulfill({ json: { items: [] } }))
  await page.route('**/api/v3/owner-authority', route => route.fulfill({ json: { goals: [], mandates: [], obligations: [], pending_propagation: 0 } }))
  await page.route('**/api/v3/initiatives', async route => {
    requested = true
    await received
    await route.fulfill({ json: { sources: [], schedules: [], initiatives: [], events: [], digests: [], attention_used_today: 0, attention_daily_limit: 20 } })
  })
  await openMindPanel(page, '/initiatives')
  await page.getByLabel('Owner control token').fill('first-fixture-proof')
  await page.getByRole('button', { name: 'Load initiatives' }).click()
  await expect.poll(() => requested).toBe(true)
  await page.getByLabel('Owner control token').fill('second-fixture-proof')
  release()
  await expect(page.getByRole('button', { name: 'Load initiatives' })).toBeEnabled()
  await expect(page.getByText('Observed work', { exact: true })).toHaveCount(0)
})
