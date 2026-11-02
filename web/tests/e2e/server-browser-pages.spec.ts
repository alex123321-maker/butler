import { expect, test } from '@playwright/test'
import { fixture, connect } from './mind-fixture'

test('owner page selection pins input and drops queued keys when the document changes', async ({ page }) => {
  await fixture(page)
  const root='a'.repeat(32),child='b'.repeat(32),rootDoc='c'.repeat(32)
  let childDoc='d'.repeat(32),epoch=0,mode='agent',sequence=0
  let releaseInput: (()=>void)|undefined
  const inputs:Record<string,unknown>[]=[]
  const jpeg=(await page.screenshot({type:'jpeg'})).toString('base64')
  await page.route('**/api/v4/server-browser',async route=>{
    const body=route.request().postDataJSON()
    if(body.operation==='list')return route.fulfill({json:{ok:true,sessions:[{browser_session_id:'session',run_id:'run',task_id:'task',mode,epoch}]}})
    if(body.operation==='take'){mode='owner';epoch++}
    if(body.operation==='release'){mode='agent';epoch++;return route.fulfill({json:{ok:true}})}
    const selected=body.page_id||root
    if(!['frame','take'].includes(body.operation)){
      inputs.push(body)
      expect(body.page_id).toBe(child);expect(body.document_id).toBe(childDoc)
      if(body.operation==='text'){
        await new Promise<void>(resolve=>{releaseInput=resolve})
        childDoc='e'.repeat(32)
      }
    }
    sequence++
    return route.fulfill({json:{ok:true,managed_pages:true,pages:[{page_id:root,document_id:rootDoc,url:'https://same.example/page',state:'ready'},{page_id:child,document_id:childDoc,url:'https://same.example/page',state:'ready'}],frame:{jpeg,frame_id:`${selected}-${sequence}`,page_id:selected,document_id:selected===root?rootDoc:childDoc,width:1280,height:720,mode,epoch,url:'https://same.example/page'}}})
  })
  await connect(page);await page.getByRole('button',{name:'Браузер',exact:true}).click()
  await expect(page.getByLabel('Страница браузера')).toHaveValue(root)
  await page.getByRole('button',{name:'Взять управление'}).click()
  await page.getByLabel('Страница браузера').selectOption(child)
  await expect(page.getByText('Вы управляете · агент ждёт')).toBeVisible()
  await expect(page.getByLabel('Страница браузера')).toBeEnabled()
  await page.getByLabel('Текст для страницы').fill('synthetic-private-input')
  await page.getByRole('button',{name:'Ввести текст'}).click()
  await expect.poll(()=>inputs.length).toBe(1)
  await page.getByRole('application',{name:'Страница серверного браузера'}).press('Enter')
  releaseInput?.()
  await expect(page.getByLabel('Текст для страницы')).toHaveValue('')
  await expect(page.getByLabel('Страница браузера')).toBeEnabled()
  await page.waitForTimeout(1000)
  expect(inputs.map(input=>input.operation)).toEqual(['text'])
  await page.getByRole('button',{name:'Вернуть агенту'}).click()
  await expect(page.getByText('Управляет агент')).toBeVisible()
})
