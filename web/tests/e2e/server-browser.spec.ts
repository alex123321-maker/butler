import { expect, test } from '@playwright/test'
import { fixture, connect, ownerToken } from './mind-fixture'

test('server browser supports viewing, exclusive control, input and explicit return without storing secrets', async ({ page }) => {
  const token = ownerToken
  await fixture(page)
  const id = '0123456789abcdef0123456789abcdef'
  const jpeg = (await page.screenshot({ type: 'jpeg' })).toString('base64')
  let mode = 'agent', epoch = 0, sequence = 0, rejectText = false
  let releaseRejectedText: (() => void) | undefined
  const inputs: Record<string, unknown>[] = []
  await page.route('**/api/v4/server-browser', async route => {
    expect(route.request().headers().authorization).toBe(`Bearer ${token}`)
    const command = route.request().postDataJSON() as Record<string, unknown>
    expect(command.owner_id).toBeUndefined(); expect(command.run_id).toBeUndefined()
    if (command.operation === 'list') return route.fulfill({ json: { ok: true, sessions: [{ browser_session_id: id, run_id: 'run-fixture', task_id: 'task-fixture', mode, epoch }] } })
    expect(command.browser_session_id).toBe(id)
    if (command.operation === 'take') { expect(command.epoch).toBe(epoch); expect(String(command.controller)).toHaveLength(32); mode = 'owner'; epoch++ }
    if (command.operation === 'release') { expect(command.epoch).toBe(epoch); mode = 'agent'; epoch++; return route.fulfill({ json: { ok: true } }) }
    if (!['frame', 'take'].includes(String(command.operation))) {
      expect(mode).toBe('owner'); expect(command.epoch).toBe(epoch); expect(command.frame_id).toBeTruthy(); inputs.push(command)
      if (command.operation === 'text' && rejectText) { await new Promise<void>(resolve => { releaseRejectedText = resolve }); return route.fulfill({ status: 409, json: { error: 'page changed' } }) }
    }
    sequence++
    return route.fulfill({ json: { ok: true, frame: { jpeg, frame_id: `frame-${sequence}`, width: 1280, height: 720, mode, epoch } } })
  })
  await connect(page)
  await page.getByRole('button', {name:'Браузер',exact:true}).click()
  await expect(page.getByAltText('Страница серверного браузера')).toBeVisible()
  await expect(page.getByText('Управляет агент')).toBeVisible()
  await page.getByRole('button', { name: 'Взять управление' }).click()
  await expect(page.getByText('Вы управляете · агент ждёт')).toBeVisible()
  await page.getByAltText('Страница серверного браузера').click({ position: { x: 100, y: 100 } })
  await expect.poll(() => inputs.filter(input => input.operation === 'click').length).toBe(1)
  await page.getByLabel('Текст для страницы').fill('owner-private-password')
  await page.getByRole('button', { name: 'Ввести текст' }).click()
  await expect.poll(() => inputs.filter(input => input.operation === 'text').length).toBe(1)
  await expect(page.getByLabel('Текст для страницы')).toHaveValue('')
  await page.getByRole('application', { name: 'Страница серверного браузера' }).press('Tab')
  await expect.poll(() => inputs.filter(input => input.operation === 'key').length).toBe(1)
  rejectText = true
  await page.getByLabel('Текст для страницы').fill('second-private-input')
  await page.getByRole('button', { name: 'Ввести текст' }).click()
  await expect.poll(() => inputs.filter(input => input.operation === 'text').length).toBe(2)
  // Queue an Enter while the preceding input is in flight. An uncertain reply
  // must discard queued keys instead of submitting against unchecked state.
  await page.getByRole('application', { name: 'Страница серверного браузера' }).press('Enter')
  releaseRejectedText?.()
  await expect(page.getByText('Команда не подтверждена или страница изменилась. Проверьте страницу перед следующим действием.')).toBeVisible()
  await page.waitForTimeout(1100)
  expect(inputs.filter(input => input.operation === 'text')).toHaveLength(2)
  expect(inputs.filter(input => input.operation === 'key')).toHaveLength(1)
  await page.getByRole('button', { name: 'Вернуть агенту' }).click()
  await expect(page.getByText('Управляет агент')).toBeVisible()
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toMatch(/fixture-browser-owner-token|owner-private-password|second-private-input/)
  await page.getByRole('dialog').getByRole('button',{name:'Закрыть',exact:true}).click()
  await page.getByRole('button',{name:'Настройки',exact:true}).first().click()
  await page.getByLabel('Доступ владельца',{exact:true}).fill('changed-proof')
  await expect(page.getByAltText('Страница серверного браузера')).toHaveCount(0)
})

test('closing a manually controlled browser keeps the agent fence and clears the frame',async({page})=>{
 await fixture(page)
 let mode='agent',epoch=0,releases=0
 const jpeg=(await page.screenshot({type:'jpeg'})).toString('base64')
 await page.route('**/api/v4/server-browser',r=>{
  const body=r.request().postDataJSON()
  if(body.operation==='list')return r.fulfill({json:{ok:true,sessions:[{browser_session_id:'session',task_id:'task-browser',run_id:'run-browser',mode,epoch}]}})
  if(body.operation==='take'){mode='owner';epoch++}
  if(body.operation==='release'){releases++;mode='agent';epoch++}
  return r.fulfill({json:{ok:true,frame:{jpeg,frame_id:'current-frame',width:1280,height:720,mode,epoch,url:'https://example.invalid'}}})
 })
 await connect(page);await page.getByRole('button',{name:'Браузер',exact:true}).click();await page.getByRole('button',{name:'Взять управление'}).click();await expect(page.getByText('Вы управляете · агент ждёт')).toBeVisible();await page.getByRole('dialog').getByRole('button',{name:'Закрыть',exact:true}).click();expect(releases).toBe(0);await expect(page.getByAltText('Страница серверного браузера')).toHaveCount(0)
 await page.getByRole('button',{name:'Браузер',exact:true}).click();await expect(page.getByText('Ручное управление — восстановите доступ')).toBeVisible();expect(releases).toBe(0)
})
