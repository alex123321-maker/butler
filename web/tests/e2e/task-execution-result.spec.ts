import { expect, test } from '@playwright/test'
import { fixture, connect, task } from './mind-fixture'
const cases = [
  {
    title: 'confirmed action with failed notification',
    effect: 'effect_confirmed',
    delivery: 'failed',
    answer: 'final',
    action: 'Эффект подтверждён',
    notice: 'Notification failed. The saved response remains available.',
  },
  {
    title: 'unknown action and notification',
    effect: 'effect_unknown',
    delivery: 'delivery_unknown',
    answer: 'final',
    action: 'Эффект неизвестен — автоматический повтор недопустим',
    notice:
      'Notification delivery is unknown. It will not be sent again automatically.',
  },
  {
    title: 'partial response available in Web',
    effect: 'effect_absent',
    delivery: 'result_ready',
    answer: 'partial',
    action: 'Подтверждено отсутствие эффекта',
    notice:
      'Response is stored and available in Web. Reading it is not tracked.',
  },
  {
    title: 'prepared action without recorded evidence',
    effect: 'prepared',
    delivery: 'pending',
    answer: 'final',
    action: 'Подготовлено, эффект не подтверждён',
    notice:
      'Notification is waiting for delivery. The saved response remains available.',
  },
  {
    title: 'removed response with failed notification',
    effect: 'effect_confirmed',
    delivery: 'failed',
    answer: 'final',
    action: 'Action evidence status is unavailable.',
    notice: 'Notification failed.',
    redacted: true,
  },
]
for (const scenario of cases)
  test('independent result states: ' + scenario.title, async ({ page }) => {
    await fixture(page, { tasks: true })
    await page.route('**/api/v2/tasks/task-mind', (route) =>
      route.fulfill({
        json: {
          task: { ...task, error_summary: '' },
          result: {
            has_result: true,
            verified: false,
            outcome_summary: 'PRIVATE_SAVED_RESPONSE',
          },
          final_result: {
            result_id: 'result',
            answer_state: scenario.answer,
            privacy_redacted: scenario.redacted || false,
            effects: [
              {
                action_id: 'action',
                effect_state: scenario.effect,
                evidence_refs:
                  scenario.effect === 'prepared' ? null : ['evidence:1'],
              },
            ],
            limitations: ['task_completion_not_verified'],
          },
          delivery: { state: scenario.delivery },
          durable_task: {
            goal: task.goal,
            constraints: [],
            completion_criteria: ['Independent tests pass'],
          },
        },
      }),
    )
    await connect(page, '/tasks/task-mind')
    const dialog = page.getByRole('dialog')
    await expect(
      dialog.getByText('Не подтверждена', { exact: true }),
    ).toBeVisible()
    if (scenario.redacted) {
      await expect(
        dialog.getByText('Содержимое ответа и доказательства недоступны.'),
      ).toBeVisible()
      await expect(dialog.getByText('PRIVATE_SAVED_RESPONSE')).toHaveCount(0)
      await expect(
        dialog.getByText('Эффект подтверждён', { exact: true }),
      ).toHaveCount(0)
    } else {
      await expect(
        dialog.getByText(scenario.action, { exact: true }),
      ).toBeVisible()
      await expect(
        dialog.getByText(
          scenario.answer === 'partial' ? 'Частичный' : 'Сохранён',
          { exact: true },
        ),
      ).toBeVisible()
      await dialog
        .getByText('Сохранённые доказательства', { exact: true })
        .click()
      await expect(
        dialog.getByText(
          scenario.effect === 'prepared'
            ? 'Ссылки на доказательства не сохранены.'
            : 'evidence:1',
          { exact: true },
        ),
      ).toBeVisible()
    }
    await dialog.getByText('Критерии готовности', { exact: true }).click()
    await expect(dialog.getByText('Independent tests pass')).toBeVisible()
    if (scenario.delivery === 'failed')
      await expect(
        dialog.getByText('Ошибка доставки', { exact: true }),
      ).toBeVisible()
    if (scenario.delivery === 'delivery_unknown')
      await expect(
        dialog.getByText('Неизвестна', { exact: true }),
      ).toBeVisible()
  })
test('unavailable retained result does not become a successful answer', async ({
  page,
}) => {
  await fixture(page, { tasks: true })
  await page.route('**/api/v2/tasks/task-mind', (route) =>
    route.fulfill({
      json: {
        task: { ...task, error_summary: '' },
        result: { has_result: false, verified: false },
        final_result: { state: 'result_read_unavailable' },
        delivery: { state: 'delivery_read_unavailable' },
      },
    }),
  )
  await connect(page, '/tasks/task-mind')
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByText('Недоступен', { exact: true })).toBeVisible()
  await expect(
    dialog.getByText('Нет подтверждения', { exact: true }),
  ).toBeVisible()
  await expect(
    dialog.getByText('Эффект подтверждён', { exact: true }),
  ).toHaveCount(0)
})
