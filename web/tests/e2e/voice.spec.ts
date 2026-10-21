import { openMindPanel } from './mind-panel'
import { expect, test } from '@playwright/test'

const token = 'fixture-voice-owner-token-000000000000'
const source = { source_id: 'reviewed-voice', revision: 1, access_revision: 1, content_hash: 'fixture-hash' }
const wav = () => ({ name: 'external.wav', mimeType: 'audio/wav', buffer: Buffer.alloc(46) })
test.use({ launchOptions: { args: ['--use-fake-ui-for-media-stream', '--use-fake-device-for-media-stream'] } })

test('explicit microphone capture shows state, releases tracks and sends bounded local PCM only after upload', async ({ page }) => {
  await page.addInitScript(() => {
    const original = navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices)
    navigator.mediaDevices.getUserMedia = async constraints => {
      const stream = await original(constraints)
      ;(window as unknown as { voiceTestStream: MediaStream }).voiceTestStream = stream
      return stream
    }
  })
  let uploads = 0
  await page.route('**/api/v3/voice/**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/status')) return route.fulfill({ json: { enabled: true } })
    const bytes = route.request().postDataBuffer()!
    expect(url.searchParams.get('role')).toBe('note')
    expect(bytes.subarray(0, 4).toString()).toBe('RIFF')
    expect(bytes.readUInt32LE(24)).toBe(16000)
    expect(bytes.readUInt16LE(22)).toBe(1)
    expect(bytes.length).toBeGreaterThan(44); expect(bytes.length).toBeLessThanOrEqual(3840044)
    uploads++
    return route.fulfill({ json: { clip_id: 'capture', role: 'note', state: 'stored', revision: 1, transcript: '' } })
  })
  await openMindPanel(page, '/voice')
  await page.getByLabel('Токен владельца').fill(token)
  await page.getByRole('button', { name: 'Проверить доступность' }).click()
  await page.getByRole('button', { name: 'Включить микрофон', exact: true }).click()
  await expect(page.getByTestId('microphone-state')).toContainText('идёт запись')
  await page.waitForTimeout(300)
  expect(uploads).toBe(0)
  await page.getByRole('button', { name: 'Остановить запись', exact: true }).click()
  await expect(page.getByTestId('microphone-state')).toHaveText('Микрофон выключен')
  expect(await page.evaluate(() => (window as unknown as { voiceTestStream: MediaStream }).voiceTestStream.getTracks().every(t => t.readyState === 'ended'))).toBe(true)
  await page.getByRole('button', { name: 'Сохранить запись на Butler' }).click()
  expect(uploads).toBe(1)
})

test('voice starts off, external and partial speech cannot confirm commands, text Stop remains available', async ({ page }) => {
  let microphoneRequests = 0
  let voiceRequests = 0
  await page.exposeFunction('voiceMicrophoneRequested', () => { microphoneRequests++ })
  await page.addInitScript(() => {
    Object.defineProperty(navigator.mediaDevices, 'getUserMedia', { value: async () => {
      await (window as unknown as { voiceMicrophoneRequested: () => Promise<void> }).voiceMicrophoneRequested()
      throw new Error('fixture_permission_denied')
    } })
  })
  const clip = { clip_id: 'external', role: 'external', state: 'stored', revision: 1, transcript: '' }
  await page.route('**/api/v3/voice/**', async route => {
    voiceRequests++
    expect(route.request().headers().authorization).toBe(`Bearer ${token}`)
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/status')) return route.fulfill({ json: { enabled: true } })
    if (url.pathname.endsWith('/clips')) { expect(url.searchParams.get('role')).toBe('external'); return route.fulfill({ json: clip }) }
    if (url.pathname.endsWith('/transcribe')) { clip.state = 'partial'; clip.transcript = 'Не отправляй…'; clip.revision++; return route.fulfill({ status: 202, json: { job_id: 'job' } }) }
    return route.fulfill({ json: clip })
  })
  let stops = 0
  await page.route('**/api/v3/tasks/current-task/control', async route => {
    expect(route.request().postDataJSON()).toEqual({ kind: 'stop', expected_revision: 7 }); stops++
    await route.fulfill({ json: { task: { status: 'cancelled', revision: 8 } } })
  })
  await openMindPanel(page, '/voice')
  await expect(page.getByTestId('microphone-state')).toHaveText('Микрофон выключен')
  await expect(page.getByRole('button', { name: 'Включить микрофон', exact: true })).toBeDisabled()
  expect(microphoneRequests).toBe(0); expect(voiceRequests).toBe(0)
  await page.getByLabel('Токен владельца').fill(token)
  await page.getByLabel('Задача (для управления обязательна)').fill('current-task')
  await page.getByLabel('Текущая версия задачи').fill('7')
  await page.getByRole('button', { name: 'Stop задачи — текстовое управление' }).click()
  expect(stops).toBe(1)
  await page.getByRole('button', { name: 'Проверить доступность' }).click()
  await page.getByRole('button', { name: 'Включить микрофон', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Микрофон недоступен')
  expect(microphoneRequests).toBe(1)
  await page.locator('input[type=file]').setInputFiles(wav())
  await page.getByRole('button', { name: 'Сохранить запись на Butler' }).click()
  await page.getByRole('button', { name: 'Распознать локально' }).click()
  await expect(page.getByLabel('Текст для просмотра')).toHaveValue('Не отправляй…')
  await page.getByLabel('Роль записи').selectOption('owner_command')
  await page.getByLabel('Я проверил имена, числа, адресата и отрицания').check()
  await page.getByLabel('Сессия', { exact: true }).fill('web:owner')
  await expect(page.getByRole('button', { name: 'Подтвердить обычный ввод' })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Подтвердить Stop / ограничение задачи' })).toBeDisabled()
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain(token)
})

test('speech failure and owner interruption preserve reviewed text', async ({ page }) => {
  const text = 'Не отправляй Ивану семнадцать файлов.'
  const clip = { clip_id: 'review', role: 'owner_command', state: 'submitted', revision: 4, transcript: text, reviewed_source: source }
  let speeches = 0
  let releaseSpeech: (() => void) | undefined
  await page.route('**/api/v3/voice/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/status')) return route.fulfill({ json: { enabled: true } })
    if (path.endsWith('/speech')) {
      expect(route.request().postDataJSON()).toEqual(source); speeches++
      if (speeches === 1) return route.fulfill({ status: 409, json: { error: 'voice_unavailable' } })
      await new Promise<void>(resolve => { releaseSpeech = resolve })
      return route.fulfill({ status: 409, json: { error: 'voice_cancelled' } }).catch(() => {})
    }
    return route.fulfill({ json: clip })
  })
  await openMindPanel(page, '/voice')
  await page.getByLabel('Токен владельца').fill(token)
  await page.getByRole('button', { name: 'Проверить доступность' }).click()
  await page.locator('input[type=file]').setInputFiles(wav())
  await page.getByRole('button', { name: 'Сохранить запись на Butler' }).click()
  // A current reload returns the stored transcript, rather than relying on local upload bytes.
  await openMindPanel(page, '/voice')
  await page.getByLabel('Токен владельца').fill(token)
  await page.getByRole('button', { name: 'Проверить доступность' }).click()
  await page.locator('input[type=file]').setInputFiles(wav())
  await page.getByRole('button', { name: 'Сохранить запись на Butler' }).click()
  await page.getByLabel('Текст для просмотра').fill(text)
  await page.getByRole('button', { name: 'Озвучить просмотренный текст' }).click()
  await expect(page.getByRole('alert')).toContainText('текст остаётся видимым')
  await expect(page.getByLabel('Текст для просмотра')).toHaveValue(text)
  await page.getByRole('button', { name: 'Озвучить просмотренный текст' }).click()
  await expect(page.getByRole('button', { name: 'Прервать озвучивание' })).toBeEnabled()
  await page.getByRole('button', { name: 'Прервать озвучивание' }).click()
  releaseSpeech?.()
  await expect(page.getByRole('button', { name: 'Прервать озвучивание' })).toBeDisabled()
  await expect(page.getByLabel('Текст для просмотра')).toHaveValue(text)
  expect(speeches).toBe(2)
})
