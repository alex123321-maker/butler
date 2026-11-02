import { expect, test } from '@playwright/test'
import { connect, fixture } from './mind-fixture'

test.use({ screenshot: 'off', trace: 'off', video: 'off' })

for (const mode of ['approve', 'recover', 'foreign_link', 'incomplete', 'cancel']) {
  test(`Google owner handoff: ${mode}`, async ({ page, context }) => {
    await fixture(page, { tasks: false })
    let state = mode === 'recover' || mode === 'cancel' ? 'approved' : 'pending'
    let approvals = 0
    let recoveries = 0
    let cancelled = false
    const card = {
      id: `extq_${'d'.repeat(64)}`, kind: 'external_login', record_id: 'cred_fixture',
      target_origin: 'https://rp.example', record_label: 'Dedicated Google',
      purpose: 'Один тестовый вход', fields: [], scope_digest: mode === 'incomplete' ? undefined : 'e'.repeat(64),
      external_scope: { rp_url: 'https://rp.example/', provider: 'google', verify_url: 'https://rp.example/auth/google/account' },
    }
    const goodLink = `https://rp.example/auth/google/start?ticket=${'a'.repeat(43)}`
    const destination = mode === 'foreign_link' ? `https://foreign.example/auth/google/start?ticket=${'a'.repeat(43)}` : goodLink
    let navigations = 0
    await context.route('https://rp.example/auth/google/start?*', async route => { navigations++; await route.fulfill({ body: '<h1>Private launch fixture</h1>', contentType: 'text/html' }) })
    await context.route('https://foreign.example/**', async route => { throw new Error(`Foreign launch attempted: ${route.request().method()}`) })
    await page.route('**/api/v4/credentials', async route => {
      const requests = cancelled ? [] : [{ ...card, state }]
      if (route.request().method() === 'GET') return route.fulfill({ json: { configured: true, requests } })
      const body = route.request().postDataJSON()
      if (body.action === 'unlock') return route.fulfill({ json: { unlock_token: 'a'.repeat(64) } })
      if (body.action === 'list') return route.fulfill({ json: { entries: [{ id: 'cred_fixture', alias: 'google.fixture', title: 'Dedicated Google', revision: 1, status: 'active' }] } })
      if (body.action === 'requests') return route.fulfill({ json: { requests } })
      expect(route.request().headers()['x-butler-vault']).toBe('a'.repeat(64))
      if (body.action === 'respond') {
        expect(body).toMatchObject({ id: card.id, record_id: card.record_id, allow: true, login_authorization: 'browser_external_google_login_once_v1', scope_digest: card.scope_digest, otp: '' })
        expect(body.password).toBeUndefined()
        approvals++
        state = 'approved'
        return route.fulfill({ json: { ok: true, launch_url: destination } })
      }
      if (body.action === 'external_launch') { recoveries++; expect(body.scope_digest).toBe(card.scope_digest); return route.fulfill({ json: { launch_url: destination } }) }
      if (body.action === 'external_cancel') { expect(body).toMatchObject({ id: card.id, scope_digest: card.scope_digest }); cancelled = true }
      return route.fulfill({ json: { ok: true } })
    })
    await connect(page)
    await page.getByRole('button', { name: 'Открыть запрос', exact: true }).click()
    for (let i = 0; i < 6; i++) await page.getByLabel(`Цифра ${i + 1}`, { exact: true }).fill('654321'[i]!)
    await page.getByRole('button', { name: 'Открыть', exact: true }).click()
    const request = page.locator('.vault-request')
    await expect(request).toContainText('Google передаст подтверждение аккаунта и email')
    await expect(request.getByRole('combobox')).toBeDisabled()
    const approve = request.getByRole('button', { name: 'Разрешить один вход через Google', exact: true })
    if (mode === 'incomplete') {
      await expect(approve).toBeDisabled()
    } else if (mode === 'cancel') {
      await request.getByRole('button', { name: 'Отменить вход', exact: true }).click()
      await expect(request).toHaveCount(0)
      expect(cancelled).toBe(true)
    } else {
      if (mode === 'recover') {
        await expect(approve).toBeDisabled()
        await request.getByRole('button', { name: 'Открыть Google в браузере', exact: true }).click()
        await expect.poll(() => recoveries).toBe(1)
      } else {
        await approve.click()
        await expect.poll(() => approvals).toBe(1)
      }
      if (mode === 'foreign_link') {
        await expect(page.locator('.vault-error')).toContainText('Ссылка для входа недоступна')
        expect(navigations).toBe(0)
      } else {
        await expect.poll(() => navigations).toBe(1)
        await expect(approve).toBeDisabled()
        expect(approvals).toBe(mode === 'recover' ? 0 : 1)
      }
    }
    for (const popup of context.pages()) if (popup !== page) await popup.close()
  })
}
