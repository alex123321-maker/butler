import { expect, test } from '@playwright/test'
import { writeFile } from 'node:fs/promises'
import { openMindPanel } from './mind-panel'

test('actual managed pages owner UI selects the second identical popup through Core Broker and Chromium',async({page})=>{
  const endpoint=process.env.BUTLER_MANAGED_PAGES_REAL_API
  const done=process.env.BUTLER_MANAGED_PAGES_UI_DONE
  test.skip(!endpoint||!done,'Requires isolated actual page Broker/PG/runtime and production owner HTTP handler')
  await page.route('**/api/v4/server-browser',async route=>{
    const response=await route.fetch({url:`${endpoint}/api/v4/server-browser`})
    await route.fulfill({response})
  })
  await openMindPanel(page,'/browser',undefined,'owner-browser-fixture-'.repeat(2))
  const select=page.getByLabel('Страница браузера')
  const image=page.getByAltText('Страница серверного браузера')
  await expect(image).toBeVisible();await expect(select.locator('option')).toHaveCount(2)
  await page.getByRole('button',{name:'Взять управление'}).click()
  await expect(page.getByText('Вы управляете · агент ждёт')).toBeVisible()
  const root=await select.inputValue()
  let box=await image.boundingBox();expect(box).toBeTruthy()
  await image.click({position:{x:40*box!.width/1280,y:100*box!.height/720}})
  await expect(select.locator('option')).toHaveCount(3)
  const ids=await select.locator('option').evaluateAll(options=>options.map(option=>(option as HTMLOptionElement).value))
  const children=ids.filter(id=>id!==root)
  expect(children).toHaveLength(2);expect(children[0]).not.toBe(children[1])
  await select.selectOption(children[1]!)
  await expect(select).toBeEnabled()
  box=await image.boundingBox();expect(box).toBeTruthy()
  await image.click({position:{x:40*box!.width/1280,y:120*box!.height/720}})
  await page.getByLabel('Текст для страницы').fill('owner-web-private-canary')
  await page.getByRole('button',{name:'Ввести текст'}).click()
  await expect(page.getByLabel('Текст для страницы')).toHaveValue('')
  await expect(select).toBeEnabled()
  box=await image.boundingBox()
  await image.click({position:{x:40*box!.width/1280,y:200*box!.height/720}})
  await expect(select).toBeEnabled()
  await expect(page.getByText('Команда не подтверждена или страница изменилась.',{exact:false})).toHaveCount(0)
  await page.getByRole('button',{name:'Вернуть агенту'}).click()
  await expect(page.getByText('Управляет агент')).toBeVisible()
  expect(await page.evaluate(()=>JSON.stringify({local:{...localStorage},session:{...sessionStorage}}))).not.toMatch(/owner-web-private-canary|owner-browser-fixture/)
  await writeFile(done!,'done',{flag:'wx'})
})
