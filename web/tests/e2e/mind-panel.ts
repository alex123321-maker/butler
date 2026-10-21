import { expect, type Page, type Route } from '@playwright/test'

// Retained privileged-workflow cases enter the continuous app through its owner
// gate. Bootstrap fixtures are removed before exercising the panel's real mocks.
export async function openMindPanel(
  page: Page,
  path: string,
  options?: Parameters<Page['goto']>[1],
  ownerProof = 'mind-panel-bootstrap-fixture-0000000000000',
) {
  const bootstrap = async (route: Route) => {
    const pathname = new URL(route.request().url()).pathname
    if (pathname === '/api/v3/communications/profile')
      return route.fulfill({ status: 404, json: { error: 'legacy_fixture' } })
    const json = pathname.endsWith('/access')
      ? { authenticated: true }
      : pathname.endsWith('/browse')
        ? { items: [], has_more: false, mode: 'recent', diagnostics: [] }
        : pathname.endsWith('/tasks')
          ? { items: [], total: 0 }
          : pathname.endsWith('/approvals')
            ? { approvals: [] }
            : { sources: [], schedules: [], digests: [] }
    await route.fulfill({ json })
  }
  const patterns = [
    '**/api/v3/communications/profile',
    '**/api/v4/memory/access',
    '**/api/v4/memory/browse?limit=6&offset=0',
    '**/api/v2/tasks?limit=100&sort=updated_at_desc',
    '**/api/v2/approvals',
    '**/api/v3/initiatives',
  ]
  for (const pattern of patterns) await page.route(pattern, bootstrap)
  await page.goto(
    path.startsWith('/memory')
      ? path.replace('/memory', '/memory/manage')
      : path === '/settings'
        ? '/settings/providers'
        : path,
    options,
  )
  await page.getByLabel('Доступ владельца', { exact: true }).fill(ownerProof)
  const loaded = Promise.all(
    patterns.map((pattern) => page.waitForResponse(pattern)),
  )
  await page.getByRole('button', { name: 'Подключить', exact: true }).click()
  await loaded
  if (path.startsWith('/tasks/')) {
    await page.getByText('Управление и доказательства', { exact: true }).click()
  }
  await expect(page.getByRole('dialog')).toBeVisible()
  if (!path.startsWith('/tasks/'))
    await expect(page.locator('.mind-advanced')).toBeVisible()
  const proof = page.getByLabel('Owner control token', { exact: true })
  if (await proof.count()) await proof.fill('')
  const voiceProof = page.getByLabel('Токен владельца', { exact: true })
  if (await voiceProof.count()) await voiceProof.fill('')
  for (const pattern of patterns) await page.unroute(pattern, bootstrap)
}
