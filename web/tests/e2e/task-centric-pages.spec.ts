import { expect, test } from '@playwright/test'
import { fixture, connect } from './mind-fixture'

for (const route of [
  { path: '/tasks', title: 'Поручения' },
  { path: '/tasks/task-mind', title: 'Подготовить сводку дня' },
  { path: '/approvals', title: 'Нужно ваше решение' },
  { path: '/artifacts', title: 'Результаты' },
  { path: '/activity', title: 'История' },
  { path: '/system', title: 'Система' },
  { path: '/memory/manage', title: 'Хранилище и политики памяти' },
  { path: '/sessions', title: 'Разговоры' },
]) {
  test(
    'bookmark opens the continuous surface: ' + route.path,
    async ({ page }) => {
      const errors: string[] = []
      page.on('pageerror', (e) => errors.push(e.message))
      await fixture(page, { tasks: true })
      await connect(page, route.path)
      await expect(
        page.getByRole('dialog', { name: route.title, exact: true }),
      ).toBeVisible()
      await expect(page.locator('.sidebar,.widget-sidebar')).toHaveCount(0)
      await page
        .getByRole('dialog')
        .getByRole('button', { name: 'Закрыть', exact: true })
        .click()
      await expect(
        page.getByRole('searchbox', { name: 'Поиск в памяти' }),
      ).toBeVisible()
      expect(errors).toEqual([])
    },
  )
}
