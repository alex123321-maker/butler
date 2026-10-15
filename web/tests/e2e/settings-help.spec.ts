import { expect, test } from '@playwright/test'

test('settings show help and escape code-owned metadata', async ({ page }) => {
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/settings') {
      await route.fulfill({ json: { components: [{ name: 'orchestrator', settings: [{ key: 'BUTLER_LOG_LEVEL', component: 'orchestrator', group: 'General', value: 'info', source: 'default', is_secret: false, requires_restart: false, allowed_values: ['info', 'debug'], description: 'Уровень детализации логов.', value_hint: '<b>Выберите info или debug.</b>', validation_status: 'valid' }] }] } })
    } else if (path === '/api/v1/providers') {
      await route.fulfill({ json: { active_provider: '', providers: [] } })
    } else if (path === '/api/v1/settings/restart') {
      await route.fulfill({ json: { components: [], suggested_command: '' } })
    } else if (path === '/api/v1/prompts/system') {
      await route.fulfill({ json: { prompt: { effective_prompt: 'Operator prompt', configured_prompt: '', enabled: true, source: 'default', available_placeholders: [] } } })
    } else {
      await route.fulfill({ json: {} })
    }
  })
  await page.goto('/settings')
  const setting = page.locator('article.setting-row').filter({ hasText: 'BUTLER_LOG_LEVEL' })
  await expect(setting.getByText('Уровень детализации логов.', { exact: true })).toBeVisible()
  await expect(setting.getByText('Допустимые значения: info, debug', { exact: true })).toBeVisible()
  await expect(setting.getByText('<b>Выберите info или debug.</b>', { exact: true })).toBeVisible()
  await expect(setting.locator('b')).toHaveCount(0)
  await setting.getByRole('button', { name: 'Edit', exact: true }).click()
  await expect(setting.locator('select')).toHaveValue('info')
})
