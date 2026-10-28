// Direct recipient/Chromium acceptance, not a native Butler evaluation.
import { createRequire } from 'node:module'
import { spawn, spawnSync } from 'node:child_process'
import { mkdtemp, readFile, mkdir, writeFile } from 'node:fs/promises'
import { resolve, join } from 'node:path'
import { createServer } from 'node:net'

const [binaryArg, packageArg, cacheArg] = process.argv.slice(2)
if (!binaryArg || !packageArg || !cacheArg) throw new Error('Expected binary, Playwright package.json and repo-local cache paths')
const binary = resolve(binaryArg)
const { chromium } = createRequire(resolve(packageArg))('playwright')
await mkdir(resolve(cacheArg), { recursive: true })
const directory = await mkdtemp(join(resolve(cacheArg), 'recipient-'))
const configPath = join(directory, 'private.json')
const journalPath = join(directory, 'recipient.jsonl')
if (spawnSync(binary, ['-init', '-config', configPath], { stdio: 'ignore', windowsHide: true }).status !== 0) throw new Error('Fixture initialization failed')
const config = JSON.parse(await readFile(configPath, 'utf8'))
const reservation = createServer()
await new Promise(resolve => reservation.listen(0, '127.0.0.1', resolve))
const port = reservation.address().port
await new Promise(resolve => reservation.close(resolve))
const origin = `http://127.0.0.1:${port}`
const child = spawn(binary, ['-config', configPath, '-journal', journalPath, '-listen', `127.0.0.1:${port}`], { stdio: 'ignore', windowsHide: true })
let browser
function check(value, message) { if (!value) throw new Error(message) }
try {
  let ready = false
  for (let i = 0; i < 50; i++) {
    try { ready = (await fetch(origin)).ok } catch {}
    if (ready) break
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  check(ready, 'Fixture did not start')
  browser = await chromium.launch({ headless: true })
  const context = await browser.newContext()
  const page = await context.newPage()
  await page.goto(origin + '/read')
  check((await page.locator('body').innerText()).includes(config.fact), 'Public reference missing')
  await page.goto(origin + '/catalog')
  await page.getByLabel('Search').fill('Arctic')
  await page.getByRole('button', { name: 'Search', exact: true }).click()
  await page.getByRole('link', { name: 'Next page' }).click()
  await page.getByRole('link', { name: 'Arctic delivery' }).click()
  check((await page.locator('body').innerText()).includes(config.record), 'Navigation reference missing')
  await page.goto(origin + '/form')
  await page.getByText('Title must be Browser baseline; quantity must be between 1 and 5.', { exact: true }).waitFor()
  await page.getByLabel('Title', { exact: true }).fill('Browser baseline')
  await page.getByLabel('Quantity').fill('3')
  await page.getByRole('button', { name: 'Save request' }).click()
  await page.getByRole('heading', { name: 'Saved', exact: true }).waitFor()
  await page.goto(origin + '/account')
  await page.getByRole('heading', { name: 'Sign in', exact: true }).waitFor()
  await page.getByLabel('Username').fill(config.username)
  await page.getByLabel('Password').fill(config.password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.getByRole('heading', { name: 'Account', exact: true }).waitFor()
  check((await page.locator('body').innerText()).includes(config.private_fact), 'Protected reference missing')
  const raw = await readFile(journalPath, 'utf8')
  check(![config.username, config.password, config.private_fact].some(value => raw.includes(value)), 'Sensitive value in recipient journal')
  const receipts = raw.trim().split('\n').map(line => JSON.parse(line))
  check(receipts.length === 5 && receipts.every((r, i) => r.accepted && r.sequence === i + 1), 'Unexpected recipient effects')
  check(receipts.map(r => r.kind).join(',') === 'read,record_read,form_submission,login,account_read', 'Unexpected effect sequence')
  const report = { scope: 'direct-chromium-recipient', native_agent: 'UNEXECUTED', browser_version: browser.version(), receipts: receipts.length, status: 'PASS' }
  await writeFile(join(directory, 'smoke.json'), JSON.stringify(report, null, 2))
  console.log(JSON.stringify(report))
} catch {
  // Playwright errors can contain DOM values; never export the underlying error.
  console.error('FAIL: direct Chromium recipient smoke; private files retained under supplied cache')
  process.exitCode = 1
} finally {
  if (browser) await browser.close()
  child.kill()
}
