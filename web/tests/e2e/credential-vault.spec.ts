import { expect, test } from '@playwright/test'
import { connect, fixture, ownerToken } from './mind-fixture'

// Synthetic input only. Secret-entry tests must not retain page/trace captures.
test.use({ screenshot: 'off', trace: 'off', video: 'off' })

for (const arrival of ['before unlock', 'after unlock']) {
test(`SSO consent shows both parties and binds the protected answer to the exact card: ${arrival}`, async ({ page }) => {
  await fixture(page, { tasks: false })
  let approved = 0
  let ready = arrival === 'before unlock'
  const request = {
    id: 'ssoq_fixture', kind: 'sso_login', record_id: 'cred_fixture',
    record_label: 'Выделенный тестовый IdP', target_origin: 'https://rp.example',
    purpose: 'Войти на RP через тестовый IdP', fields: ['username', 'password'],
    scope_digest: 'd'.repeat(64),
    sso_scope: {
      rp_url: 'https://rp.example/', idp_url: 'https://idp.example/authorize',
      target_url: 'https://idp.example/login', verify_url: 'https://rp.example/account',
      rp_page: { page_id: 'a'.repeat(32), document_id: 'b'.repeat(32), observation_version: 7 },
    },
  }
  await page.route('**/api/v4/credentials', async route => {
    const pending = approved || !ready ? [] : [request]
    if (route.request().method() === 'GET')
      return route.fulfill({ json: { configured: true, requests: pending } })
    const body = route.request().postDataJSON()
    if (body.action === 'unlock') return route.fulfill({ json: { unlock_token: 'a'.repeat(64) } })
    if (body.action === 'list') return route.fulfill({ json: { entries: [{ id: 'cred_fixture', alias: 'test.idp', title: 'Выделенный тестовый IdP', revision: 1, status: 'active' }] } })
    if (body.action === 'requests') return route.fulfill({ json: { requests: pending } })
    if (body.action === 'respond') {
      expect(body).toMatchObject({ id: request.id, record_id: request.record_id, allow: true, login_authorization: 'browser_sso_login_once_v1', scope_digest: request.scope_digest, otp: '' })
      expect(body.password).toBeUndefined()
      expect(body.username).toBeUndefined()
      expect(route.request().headers()['x-butler-vault']).toBe('a'.repeat(64))
      approved++
    }
    return route.fulfill({ json: { ok: true } })
  })
  await connect(page)
  if (ready) {
    await expect(page.getByText('Запрос разрешения на один SSO-вход', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Открыть запрос', exact: true }).click()
  } else {
    await page.getByRole('button', { name: 'Учётные данные', exact: true }).click()
  }
  for (let i = 0; i < 6; i++) await page.getByLabel(`Цифра ${i + 1}`, { exact: true }).fill('654321'[i]!)
  await page.getByRole('button', { name: 'Открыть', exact: true }).click()
  await expect(page.locator('.vault-panel')).toBeVisible()
  ready = true
  const card = page.locator('.vault-request')
  await expect(card).toContainText('Сайт (RP): https://rp.example/', { timeout: 15000 })
  await expect(card).toContainText('Провайдер входа (IdP): https://idp.example/authorize')
  await expect(card).toContainText('https://idp.example/login')
  await expect(card).toContainText('https://rp.example/account')
  await expect(card).toContainText('наблюдение 7')
  await expect(card.getByRole('combobox')).toBeDisabled()
  await expect(card.getByRole('combobox')).toHaveValue(request.record_id)
  expect(approved).toBe(0)
  await expect(card.getByRole('button', { name: 'Создать запись', exact: true })).toHaveCount(0)
  await card.getByRole('button', { name: 'Разрешить один SSO-вход', exact: true }).click()
  await expect(card).toHaveCount(0)
  expect(approved).toBe(1)
})
}

test('SSO card without its exact scope cannot offer approval', async ({ page }) => {
  await fixture(page, { tasks: false })
  const request = { id: 'ssoq_incomplete', kind: 'sso_login', record_id: 'cred_fixture', target_origin: 'https://rp.example', purpose: 'Incomplete SSO card', fields: ['username', 'password'] }
  await page.route('**/api/v4/credentials', async route => {
    if (route.request().method() === 'GET') return route.fulfill({ json: { configured: true, requests: [request] } })
    const body = route.request().postDataJSON()
    if (body.action === 'unlock') return route.fulfill({ json: { unlock_token: 'a'.repeat(64) } })
    return route.fulfill({ json: { entries: [{ id: 'cred_fixture', alias: 'test.idp', title: 'Test IdP' }], requests: [request] } })
  })
  await connect(page)
  await page.getByRole('button', { name: 'Открыть запрос', exact: true }).click()
  for (let i = 0; i < 6; i++) await page.getByLabel(`Цифра ${i + 1}`, { exact: true }).fill('654321'[i]!)
  await page.getByRole('button', { name: 'Открыть', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Разрешить один SSO-вход', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Отказать', exact: true })).toBeEnabled()
})

test('one login consent pins the account and includes submit plus access verification', async ({page}) => {
  await fixture(page, {tasks: false})
  let approved = false
  const request = {id:'creq_login',kind:'login',record_id:'cred_fixture',record_label:'Личный магазин',target_origin:'https://shop.example',purpose:'Войти и прочитать заказ',fields:['username','password'],state:'pending',login_scope:{target_url:'https://shop.example/session'}}
  await page.route('**/api/v4/credentials', async route => {
    if(route.request().method()==='GET') return route.fulfill({json:{configured:true,requests:approved?[]:[request],entries:[{id:'cred_fixture',title:'Личный магазин',alias:'shop.personal',revision:1,status:'active'}]}})
    const body=route.request().postDataJSON()
    if(body.action==='unlock') return route.fulfill({json:{unlock_token:'a'.repeat(64)}})
    if(body.action==='list') return route.fulfill({json:{entries:[{id:'cred_fixture',title:'Личный магазин',alias:'shop.personal',revision:1,status:'active'}]}})
    if(body.action==='requests') return route.fulfill({json:{requests:approved?[]:[request]}})
    if(body.action==='respond') {
      expect(body.id).toBe('creq_login')
      expect(body.record_id).toBe('cred_fixture')
      expect(body.allow).toBe(true)
      expect(body.login_authorization).toBe('browser_login_once_v1')
      expect(route.request().headers()['x-butler-vault']).toBe('a'.repeat(64))
      approved=true
    }
    return route.fulfill({json:{ok:true}})
  })
  await connect(page)
  await expect(page.getByText('Запрос разрешения на один вход',{exact:true})).toBeVisible()
  await page.getByRole('button',{name:'Открыть запрос',exact:true}).click()
  for(let i=0;i<6;i++) await page.getByLabel(`Цифра ${i+1}`,{exact:true}).fill('654321'[i]!)
  await page.getByRole('button',{name:'Открыть',exact:true}).click()
  const card=page.locator('.vault-request')
  await expect(card.getByRole('combobox')).toBeDisabled()
  await expect(card).toContainText('отправить форму на https://shop.example/session и проверить доступ')
  await expect(card.getByRole('button',{name:'Создать запись',exact:true})).toHaveCount(0)
  await card.getByRole('button',{name:'Разрешить один вход',exact:true}).click()
  await expect.poll(()=>approved).toBe(true)
})

test('bookmark creates a six-cell PIN, saves separate account fields and clears on lock', async ({
  page,
}) => {
  await fixture(page, { tasks: false })
  let configured = false
  let saved: Record<string, unknown> | undefined
  let unlocked = false
  await page.route('**/api/v4/credentials', async (route) => {
    expect(route.request().headers().authorization).toBe(`Bearer ${ownerToken}`)
    if (route.request().method() === 'GET') {
      await route.fulfill({ json: { configured, requests: [] } })
      return
    }
    const body = route.request().postDataJSON()
    if (body.action === 'unlock') {
      expect(body.pin).toBe('654321')
      expect(body.confirm).toBe('654321')
      configured = true
      unlocked = true
      await route.fulfill({ json: { unlock_token: 'a'.repeat(64) } })
      return
    }
    expect(route.request().headers()['x-butler-vault']).toBe('a'.repeat(64))
    if (body.action === 'save') {
      saved = body.entry
      expect(saved?.username).toBe('fixture@example.com')
      expect(saved?.password).toBe('synthetic-private-password')
      await route.fulfill({
        json: {
          entry: {
            id: 'cred_fixture',
            ...body.entry,
            username: undefined,
            password: undefined,
            revision: 1,
            status: 'active',
          },
        },
      })
      return
    }
    if (body.action === 'lock') unlocked = false
    await route.fulfill({ json: { entries: [], requests: [], ok: true } })
  })
  await connect(page)
  await page
    .getByRole('button', { name: 'Учётные данные', exact: true })
    .click()
  await expect(
    page.getByRole('heading', { name: 'Создайте PIN' }),
  ).toBeVisible()
  const fillPin = async () => {
    for (let i = 0; i < 6; i++)
      await page
        .getByLabel(`Цифра ${i + 1}`, { exact: true })
        .fill('654321'[i]!)
  }
  await expect(page.locator('.vault-pin-cells input')).toHaveCount(6)
  await fillPin()
  await page.getByRole('button', { name: 'Продолжить', exact: true }).click()
  await expect(
    page.getByRole('heading', { name: 'Повторите PIN' }),
  ).toBeVisible()
  await fillPin()
  await page.getByRole('button', { name: 'Сохранить PIN', exact: true }).click()
  await page.getByRole('button', { name: 'Добавить', exact: true }).click()
  await page.getByLabel('Название', { exact: true }).fill('Личный магазин')
  await page.getByLabel('Alias', { exact: true }).fill('shop.personal')
  await page.getByLabel('Логин', { exact: true }).fill('fixture@example.com')
  await page
    .getByLabel('Пароль', { exact: true })
    .fill('synthetic-private-password')
  await page.getByRole('button', { name: 'Сохранить', exact: true }).click()
  await expect.poll(() => Boolean(saved)).toBe(true)
  expect(saved?.username).not.toBe(saved?.password)
  expect(
    await page.evaluate(() =>
      JSON.stringify({ ...localStorage, ...sessionStorage }),
    ),
  ).not.toContain('synthetic-private-password')
  await page.getByRole('button', { name: 'Заблокировать', exact: true }).click()
  await expect(page.locator('.vault-panel')).toHaveCount(0)
  await expect.poll(() => unlocked).toBe(false)
  await expect(
    page.locator('input[value="synthetic-private-password"]'),
  ).toHaveCount(0)
})

test('credential access and creation are distinct cards without secret text answers', async ({
  page,
}) => {
  await fixture(page, { tasks: false })
  const request = {
    id: 'creq_fixture',
    kind: 'access',
    record_id: 'cred_fixture',
    target_origin: 'https://different.example',
    purpose: 'Войти для просмотра заказов',
    fields: ['username', 'password'],
    state: 'pending',
  }
  await page.route('**/api/v4/credentials', async (route) =>
    route.fulfill({ json: { configured: true, requests: [request] } }),
  )
  await connect(page)
  await expect(
    page.getByText('Запрос доступа к учётным данным', { exact: true }),
  ).toBeVisible()
  await expect(
    page.getByText('https://different.example', { exact: true }),
  ).toBeVisible()
  request.kind = 'create'
  await page.reload()
  await connect(page)
  await expect(
    page.getByText('Нужно создать учётные данные', { exact: true }),
  ).toBeVisible()
  await page
    .getByRole('button', { name: 'Создать или выбрать', exact: true })
    .click()
  await expect(page.getByRole('heading', { name: 'Введите PIN' })).toBeVisible()
})

test('OTP input belongs to its request and SMS routing requires explicit owner configuration', async ({
  page,
}) => {
  await fixture(page, { tasks: false })
  let pending = ['one', 'two'].map((id) => ({
    id: `creq_${id}`,
    kind: 'otp',
    record_id: 'cred_fixture',
    target_origin: `https://${id}.example`,
    purpose: `Вход ${id}`,
    fields: ['otp'],
    state: 'pending',
  }))
  let answered = false
  let routed = false
  await page.route('**/api/v4/credentials', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({ json: { configured: true, requests: pending } })
      return
    }
    const body = route.request().postDataJSON()
    if (body.action === 'unlock') {
      await route.fulfill({ json: { unlock_token: 'a'.repeat(64) } })
      return
    }
    expect(route.request().headers()['x-butler-vault']).toBe('a'.repeat(64))
    if (body.action === 'respond') {
      expect(body.id).toBe('creq_two')
      expect(body.record_id).toBe('')
      expect(body.otp).toBe('739284')
      answered = true
      pending = pending.filter((r) => r.id !== body.id)
    }
    if (body.action === 'sms_route') {
      expect(body.sms_route).toMatchObject({
        record_id: 'cred_fixture',
        device_id: 'phone',
        package_name: 'sms.app',
        sender: 'Example',
        enabled: true,
      })
      routed = true
    }
    await route.fulfill({
      json: {
        ok: true,
        requests: pending,
        entries: [
          {
            id: 'cred_fixture',
            alias: 'personal',
            title: 'Личный',
            revision: 1,
            status: 'active',
            site_hint: '',
            account_label: '',
          },
        ],
        routes: [],
        devices: [{ device_id: 'phone', name: 'Тестовый телефон' }],
      },
    })
  })
  await connect(page)
  await page
    .getByRole('button', { name: 'Открыть запрос', exact: true })
    .click()
  for (let i = 0; i < 6; i++)
    await page.getByLabel(`Цифра ${i + 1}`, { exact: true }).fill('654321'[i]!)
  await page.getByRole('button', { name: 'Открыть', exact: true }).click()
  const cards = page.locator('.vault-request')
  await cards.nth(0).getByLabel('Одноразовый код').fill('111111')
  await cards.nth(1).getByLabel('Одноразовый код').fill('739284')
  await cards
    .nth(1)
    .getByRole('button', { name: 'Передать код', exact: true })
    .click()
  await expect.poll(() => answered).toBe(true)
  await page.getByRole('button', { name: 'SMS-коды', exact: true }).click()
  await page.getByRole('checkbox').check()
  await page.getByLabel('Телефон', { exact: true }).selectOption('phone')
  await page.getByLabel('Приложение SMS', { exact: true }).fill('sms.app')
  await page
    .getByLabel('Точный отправитель SMS', { exact: true })
    .fill('Example')
  await page
    .getByRole('button', { name: 'Сохранить привязку', exact: true })
    .click()
  await expect.poll(() => routed).toBe(true)
  await page.getByRole('button', { name: 'Заблокировать', exact: true }).click()
  await expect(page.getByLabel('Одноразовый код')).toHaveCount(0)
  const persisted = await page.evaluate(() =>
    JSON.stringify({ ...localStorage, ...sessionStorage }),
  )
  expect(persisted).not.toContain('739284')
  expect(persisted).not.toContain('111111')
})
