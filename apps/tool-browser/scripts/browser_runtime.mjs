import { chromium } from 'playwright'

const raw = process.argv[2]
if (!raw) {
  console.error('missing browser request payload')
  process.exit(1)
}

const request = JSON.parse(raw)
const browser = await chromium.launch({ headless: true })

try {
  const page = await browser.newPage()
  await page.goto(request.url, { waitUntil: 'domcontentloaded', timeout: 15000 })

  const title = await page.title()
  const finalURL = page.url()
  let text = ''

  if (request.tool_name === 'browser.snapshot') {
    text = await page.locator('body').innerText().catch(() => '')
  }

  process.stdout.write(JSON.stringify({ final_url: finalURL, title, text }))
} finally {
  await browser.close()
}
