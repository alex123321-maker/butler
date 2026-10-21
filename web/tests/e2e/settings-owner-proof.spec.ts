import { openMindPanel } from './mind-panel'
import { expect, test } from '@playwright/test'

test('system prompt writes and preview use owner proof only in page memory', async ({ page }) => {
  const token = 'fixture-maintenance-owner-token-00000000'
  const prompt = { configured_prompt: 'Operator instructions', effective_prompt: 'Operator instructions', enabled: true, source: 'db', available_placeholders: [] }
  let updates = 0
  let previews = 0
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/prompts/system/preview' || (path === '/api/v1/prompts/system' && route.request().method() === 'PUT')) {
      if (route.request().headers().authorization !== `Bearer ${token}`) {
        await route.fulfill({ status: 401, json: { error: 'owner authorization required' } })
        return
      }
      if (path.endsWith('/preview')) {
        previews++
        await route.fulfill({ json: { preview: { prompt, final_prompt: 'Reviewed preview', sections: [], unknown_placeholders: [], truncated: false } } })
      } else {
        updates++
        expect(route.request().postDataJSON().base_prompt).toBe('Reviewed operator instructions')
        await route.fulfill({ json: { prompt } })
      }
    } else if (path === '/api/v1/settings') {
      await route.fulfill({ json: { components: [] } })
    } else if (path === '/api/v1/settings/restart') {
      await route.fulfill({ json: { components: [], suggested_command: '' } })
    } else if (path === '/api/v1/providers') {
      await route.fulfill({ json: { active_provider: '', providers: [] } })
    } else if (path === '/api/v1/prompts/system') {
      await route.fulfill({ json: { prompt } })
    } else {
      await route.fulfill({ json: {} })
    }
  })
  await openMindPanel(page, '/settings')
  await page.getByLabel('Owner control token').fill(token)
  await page.getByPlaceholder('Write the operator base prompt here').fill('Reviewed operator instructions')
  await page.getByRole('button', { name: 'Save prompt', exact: true }).click()
  await expect(page.getByText('Prompt settings saved.', { exact: true })).toBeVisible()
  expect(updates).toBe(1)
  expect(previews).toBeGreaterThan(0)
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain(token)
  await openMindPanel(page, '/settings')
  await expect(page.getByLabel('Owner control token')).toHaveValue('')
})
