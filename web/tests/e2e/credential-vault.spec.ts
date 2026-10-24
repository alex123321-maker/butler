import { expect, test } from '@playwright/test'
import { connect, fixture, ownerToken } from './mind-fixture'

// Synthetic input only. Secret-entry tests must not retain page/trace captures.
test.use({ screenshot: 'off', trace: 'off', video: 'off' })

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
