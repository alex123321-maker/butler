import { createRequire } from 'node:module'

const require = createRequire(new URL('../../web/package.json', import.meta.url))
const { chromium } = require('@playwright/test')
const baseURL = process.env.BUTLER_LAB_SMOKE_BASE_URL || 'http://web:3000'
const browser = await chromium.launch({ headless: true })
try {
  const page = await browser.newPage()
  const failures = []
  page.on('pageerror', () => failures.push('uncaught page error'))
  page.on('requestfailed', (request) => {
    if (new URL(request.url()).pathname.startsWith('/api/')) failures.push('API connection failure')
  })
  page.on('response', (response) => {
    if (new URL(response.url()).pathname.startsWith('/api/') && response.status() >= 400) {
      failures.push(`API HTTP ${response.status()} ${new URL(response.url()).pathname}`)
    }
  })
  for (const [path, title] of [['/', 'Overview'], ['/tasks', 'Tasks'], ['/approvals', 'Approvals'], ['/memory', 'Memory']]) {
    await page.goto(new URL(path, baseURL).href)
    await page.getByRole('heading', { name: title, exact: true }).waitFor()
    await page.waitForLoadState('networkidle')
    console.log(`${path}: page and API loaded`)
  }
  await page.goto(baseURL)
  await page.getByRole('heading', { name: 'Overview', exact: true }).waitFor()
  await page.screenshot({ path: process.env.BUTLER_LAB_SMOKE_SCREENSHOT || '/cache/lab-ui.png', fullPage: true })
  if (failures.length) throw new Error(`Live lab smoke failed: ${failures.join(', ')}`)
  console.log('Live lab browser smoke passed')
} finally {
  await browser.close()
}
