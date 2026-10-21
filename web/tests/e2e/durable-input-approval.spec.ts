import { expect, test } from '@playwright/test'
import { fixture, connect, ownerToken } from './mind-fixture'

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(Crypto.prototype, 'randomUUID', {
      value: undefined,
      configurable: true,
    })
  })
  await fixture(page)
})

test('guarded tabs retain distinct origins, exact selection and explicit revoke', async ({
  page,
}) => {
  const id = 'a'.repeat(48),
    first = 'b'.repeat(48),
    second = 'c'.repeat(48)
  let selected = false,
    revoked = false
  const deliveries: string[] = []
  await page.route('**/api/v2/approvals', (r) =>
    r.fulfill({
      json: {
        approvals: [
          {
            approval_id: id,
            status: selected ? 'approved' : 'pending',
            run_id: 'waiting-run',
            tool_name: 'single_tab.bind.guarded',
            summary: 'Выбрать вкладку',
          },
        ],
      },
    }),
  )
  await page.route('**/api/v3/tab-choices/' + id, (r) => {
    expect(r.request().headers().authorization).toBe('Bearer ' + ownerToken)
    return r.fulfill({
      json: {
        approval_id: id,
        state: selected ? 'selected' : 'pending',
        expires_at: '2030-01-01T00:00:00Z',
        candidates: [
          {
            candidate_token: first,
            origin: 'https://same.example',
            ordinal: 1,
            active_at_request: true,
            selected: false,
          },
          {
            candidate_token: second,
            origin: 'https://same.example',
            ordinal: 2,
            active_at_request: false,
            selected,
          },
        ],
      },
    })
  })
  await page.route('**/api/v3/tab-choices/' + id + '/select', (r) => {
    expect(r.request().postDataJSON()).toEqual({ candidate_token: second })
    deliveries.push(r.request().headers()['x-butler-delivery-id'])
    selected = true
    return r.fulfill({ json: { single_tab_session_id: 'opaque-session' } })
  })
  await page.route('**/api/v3/tab-choices/' + id + '/revoke', (r) => {
    expect(r.request().postDataJSON()).toEqual({})
    deliveries.push(r.request().headers()['x-butler-delivery-id'])
    revoked = true
    return r.fulfill({ json: { revoked: true } })
  })
  await connect(page, '/approvals')
  const panel = page.getByRole('dialog')
  await expect(
    panel.getByRole('button', { name: 'Tab 1 · https://same.example' }),
  ).toBeVisible()
  await panel
    .getByRole('button', { name: 'Tab 2 · https://same.example' })
    .click()
  await expect(
    panel.getByText('Tab selected. No browser action has been executed.'),
  ).toBeVisible()
  await expect(
    panel.getByRole('button', { name: 'Tab 2 · https://same.example' }),
  ).toBeDisabled()
  await panel.getByRole('button', { name: 'Revoke tab binding' }).click()
  await expect(
    panel.getByText(
      'Tab binding revoked. Previously recorded effects are retained.',
    ),
  ).toBeVisible()
  expect(revoked).toBe(true)
  expect(deliveries).toHaveLength(2)
  expect(new Set(deliveries).size).toBe(2)
  expect(deliveries.every((x) => /^[a-f0-9]{32}$/.test(x))).toBe(true)
})

test('a changed global owner fences a late guarded-tab choice', async ({
  page,
}) => {
  const id = 'd'.repeat(48)
  let release: () => void = () => {}
  const held = new Promise<void>((r) => {
    release = r
  })
  await page.route('**/api/v2/approvals', (r) =>
    r.fulfill({
      json: {
        approvals: [
          {
            approval_id: id,
            status: 'pending',
            run_id: 'waiting-run',
            tool_name: 'single_tab.bind.guarded',
          },
        ],
      },
    }),
  )
  await page.route('**/api/v3/tab-choices/' + id, async (r) => {
    if (r.request().headers().authorization === 'Bearer old-proof') {
      await held
      return r.fulfill({
        json: {
          approval_id: id,
          state: 'pending',
          expires_at: '2030-01-01T00:00:00Z',
          candidates: [
            {
              candidate_token: 'e'.repeat(48),
              origin: 'https://late.example',
              ordinal: 1,
            },
          ],
        },
      })
    }
    return r.fulfill({
      status: 409,
      json: { error: 'current authority required' },
    })
  })
  await connect(page, '/approvals', 'old-proof')
  await expect(page.getByText('Loading current tab choices...')).toBeVisible()
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Закрыть', exact: true })
    .click()
  await page
    .getByRole('button', { name: 'Настройки', exact: true })
    .first()
    .click()
  await page.getByLabel('Доступ владельца', { exact: true }).fill('new-proof')
  await page.getByRole('button', { name: 'Подключить', exact: true }).click()
  await expect(
    page.getByText(
      'Current tab choices are unavailable. No binding was changed.',
    ),
  ).toBeVisible()
  release()
  await expect(
    page.getByRole('button', { name: 'Tab 1 · https://late.example' }),
  ).toHaveCount(0)
})

test('approval requires service-derived consequences and sends the exact saved identity', async ({
  page,
}) => {
  let decided = false
  await page.route('**/api/v2/approvals', (r) =>
    r.fulfill({
      json: {
        approvals: decided
          ? []
          : [
              {
                approval_id: 'approval-stable',
                status: 'pending',
                run_id: 'run-stable',
                args_json: 'PRIVATE_ARGS',
                summary: 'Submit body one',
                expires_at: '2030-01-01T00:00:00Z',
                owner_facts: {
                  summary: 'Submit body one',
                  effect: 'commit',
                  resource: 'https://example.invalid',
                  task_revision: 2,
                  constraints_revision: 1,
                  contract_hash: 'a'.repeat(64),
                  args_hash: 'b'.repeat(64),
                },
              },
            ],
      },
    }),
  )
  await page.route('**/api/v2/approvals/approval-stable/approve', (r) => {
    expect(r.request().headers().authorization).toBe('Bearer ' + ownerToken)
    expect(r.request().headers()['x-butler-delivery-id']).toMatch(
      /^[a-f0-9]{32}$/,
    )
    decided = true
    return r.fulfill({ json: { changed: true } })
  })
  await connect(page, '/approvals')
  await expect(page.getByText('PRIVATE_ARGS')).toHaveCount(0)
  await page.getByRole('button', { name: 'Разрешить', exact: true }).click()
  await expect(page.getByText('Ожидающих запросов нет.')).toBeVisible()
  expect(decided).toBe(true)
})

test('receipt processing is observed independently of task verification', async ({
  page,
}) => {
  let reads = 0
  await page.route('**/api/v3/inputs/input-stable', (r) =>
    r.fulfill({
      json: {
        input_id: 'input-stable',
        session_key: 'web:owner',
        run_id: 'run-stable',
        status: ++reads < 2 ? 'queued' : 'completed',
        checkpoint: 'finalizing',
      },
    }),
  )
  await connect(page, '/inputs/input-stable')
  await expect(page.getByTestId('receipt-status')).toHaveText('queued')
  await expect(page.getByTestId('receipt-status')).toHaveText('completed')
  await expect(
    page.getByText('Готовность поручения проверяется отдельно.', {
      exact: false,
    }),
  ).toBeVisible()
  await expect(
    page.getByRole('link', { name: 'Открыть попытку' }),
  ).toHaveAttribute('href', '/runs/run-stable')
})
