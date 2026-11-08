// Operator-owned acceptance driver. No request interception or mocked API.
// Input: {endpoint,task_id,owner_token_file,action?:'stop'|'resume'}; then {execute:true}.
// The proof stays in process memory, never command arguments, output or storage.
import fs from 'node:fs'
import readline from 'node:readline'
import { chromium } from '../../apps/tool-browser/node_modules/playwright/index.mjs'

const lines = readline.createInterface({ input: process.stdin })
const iterator = lines[Symbol.asyncIterator]()
let browser
let stage = 'input'
try {
  const config = JSON.parse((await iterator.next()).value)
  if (!/^https?:\/\//.test(config.endpoint) || !/^[a-zA-Z0-9_-]{1,160}$/.test(config.task_id)) throw Error()
  const action = config.action || 'stop'
  if (!['stop', 'resume'].includes(action)) throw Error()
  const token = fs.readFileSync(config.owner_token_file, 'utf8').trim()
  if (token.length < 32) throw Error()
  stage = 'launch'
  browser = await chromium.launch({ headless: true })
  const page = await browser.newPage()
  page.setDefaultTimeout(15000)
  stage = 'owner_gate'
  await page.goto(config.endpoint.replace(/\/$/, '') + '/tasks/' + encodeURIComponent(config.task_id))
  await page.getByLabel('Доступ владельца', { exact: true }).fill(token)
  await page.getByRole('button', { name: 'Подключить', exact: true }).click()
  stage = 'task_panel'
  await page.getByText('Управление и доказательства', { exact: true }).click()
  await page.getByLabel('Owner control token', { exact: true }).fill(token)
  const stop = page.getByRole('button', { name: action === 'stop' ? 'Stop task' : 'Resume task', exact: true })
  await stop.waitFor({ state: 'visible' })
  if (!await stop.isEnabled()) throw Error()
  stage = 'signal'
  process.stdout.write(JSON.stringify({ owner_ui_ready: true }) + '\n')
  const signal = JSON.parse((await iterator.next()).value)
  if (signal.execute !== true && !(action === 'stop' && signal.stop === true)) throw Error()
  const began = new Date().toISOString()
  const response = page.waitForResponse(r => r.url().endsWith('/api/v3/tasks/' + config.task_id + '/control') && r.request().method() === 'POST')
  stage = 'control'
  await stop.click()
  const ack = await response
  if (ack.status() !== 200) throw Error()
  stage = 'outcome'
  const body = await ack.json()
  if (body.task?.status !== (action === 'stop' ? 'cancelled' : 'active')) throw Error()
  const persisted = await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))
  if (persisted.includes(token)) throw Error()
  process.stdout.write(JSON.stringify({ action, owner_ui_control_acked: true, began_at: began, acked_at: new Date().toISOString(), task_revision: body.task.revision, continuation_input_id: body.continuation_input_id || null, proof_persisted: false }) + '\n')
} catch {
  process.stderr.write(JSON.stringify({ owner_ui_control_failed: true, stage }) + '\n')
  process.exitCode = 1
} finally {
  await browser?.close()
  lines.close()
}
