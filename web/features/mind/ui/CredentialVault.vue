<template>
  <button
    v-if="ownerToken"
    class="vault-bookmark"
    type="button"
    aria-label="Учётные данные"
    @click="openVault"
  >
    <svg viewBox="0 0 24 30" aria-hidden="true">
      <path d="M3 2h18v25l-9-6-9 6z" />
    </svg>
    <span v-if="requests.length" class="vault-badge">{{
      requests.length
    }}</span>
  </button>
  <Teleport to="body">
    <section
      v-if="requests.length && !pinOpen && !panelOpen"
      class="vault-prompt"
      aria-live="polite"
    >
      <strong>{{
        requests[0]?.kind === 'create'
          ? 'Нужно создать учётные данные'
          : requests[0]?.kind === 'otp'
            ? 'Нужен одноразовый код'
            : requests[0]?.kind === 'login'
              ? 'Запрос разрешения на один вход'
              : 'Запрос доступа к учётным данным'
      }}</strong>
      <p>{{ requests[0]?.target_origin }}</p>
      <p v-if="requests[0]?.record_label">{{ requests[0]?.record_label }}</p>
      <p>{{ requests[0]?.purpose }}</p>
      <button class="vault-primary" type="button" @click="openVault">
        {{
          requests[0]?.kind === 'create'
            ? 'Создать или выбрать'
            : 'Открыть запрос'
        }}
      </button>
    </section>
    <div
      v-if="pinOpen"
      class="vault-overlay"
      @click.self="close"
      @keydown.esc.stop.prevent="close"
      @keydown.tab="trapFocus"
    >
      <form
        class="vault-pin-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="vault-pin-title"
        @submit.prevent="submitPin"
      >
        <button
          class="vault-close"
          type="button"
          aria-label="Закрыть"
          @click="close"
        >
          ×
        </button>
        <h2 id="vault-pin-title">
          {{
            configured && !changingPin
              ? 'Введите PIN'
              : confirmation
                ? 'Повторите PIN'
                : changingPin
                  ? 'Новый PIN'
                  : 'Создайте PIN'
          }}
        </h2>
        <p>Учётные данные</p>
        <div class="vault-pin-cells" @paste.prevent="pastePin">
          <input
            v-for="(_, index) in digits"
            :key="index"
            :ref="(el) => setCell(el, index)"
            :value="digits[index]"
            type="password"
            inputmode="numeric"
            pattern="[0-9]*"
            maxlength="1"
            autocomplete="off"
            :aria-label="`Цифра ${index + 1}`"
            @input="inputDigit($event, index)"
            @keydown="keyDigit($event, index)"
          />
        </div>
        <p v-if="error" role="alert" class="vault-error">{{ error }}</p>
        <button
          class="vault-primary"
          type="submit"
          :disabled="busy || digits.some((d) => !d)"
        >
          {{
            configured && !changingPin
              ? 'Открыть'
              : confirmation
                ? 'Сохранить PIN'
                : 'Продолжить'
          }}
        </button>
      </form>
    </div>
    <div
      v-if="panelOpen"
      class="vault-overlay vault-panel-overlay"
      @click.self="close"
      @keydown.esc.stop.prevent="close"
      @keydown.tab="trapFocus"
    >
      <aside
        class="vault-panel"
        role="dialog"
        aria-modal="true"
        aria-labelledby="vault-title"
      >
        <header>
          <h2 id="vault-title">Учётные данные</h2>
          <button type="button" aria-label="Заблокировать" @click="lock">
            Заблокировать</button
          ><button type="button" aria-label="Закрыть" @click="close">×</button>
        </header>
        <p class="vault-muted">
          Секреты передаются сайту после вашего разрешения.
        </p>
        <button type="button" @click="beginPinChange">Сменить PIN</button>
        <p v-if="error" role="alert" class="vault-error">{{ error }}</p>
        <section
          v-for="request in requests"
          :key="request.id"
          class="vault-request"
        >
          <strong>{{
            request.kind === 'create'
              ? 'Создать учётные данные'
              : request.kind === 'otp'
                ? 'Нужен одноразовый код'
                : request.kind === 'login' ? 'Разрешить вход' : 'Разрешить доступ'
          }}</strong>
          <p>{{ request.target_origin }}</p>
          <p v-if="request.record_label">{{ request.record_label }}</p>
          <p>{{ request.purpose }}</p>
          <p class="vault-muted">Поля: {{ request.fields.join(', ') }}</p>
          <p v-if="request.kind === 'login'" class="vault-muted">
            Один вход: подставить учётные данные, отправить форму на
            {{ request.login_scope?.target_url }} и проверить доступ.
            Другие отправки не разрешены.
          </p>
          <template v-if="request.kind === 'otp'"
            ><input
              v-model="otp[request.id]"
              type="password"
              autocomplete="one-time-code"
              aria-label="Одноразовый код"
            /><button type="button" @click="respond(request, true)">
              Передать код
            </button></template
          >
          <template v-else
            ><select
              v-model="selectedRecords[request.id]"
              :disabled="request.kind === 'login'"
              :aria-label="`Запись для ${request.target_origin}`"
            >
              <option value="">Выберите запись</option>
              <option
                v-for="entry in entries"
                :key="entry.id"
                :value="entry.id"
              >
                {{ entry.title }} · {{ entry.alias }}
              </option></select
            ><button
              type="button"
              :disabled="!selectedRecords[request.id]"
              @click="respond(request, true)"
            >
              {{ request.kind === 'login' ? 'Разрешить один вход' : 'Разрешить' }}</button
            ><button v-if="request.kind !== 'login'" type="button" @click="newEntry(request)">
              Создать запись
            </button></template
          >
          <button type="button" @click="respond(request, false)">
            Отказать
          </button>
        </section>
        <div class="vault-search">
          <input
            v-model="query"
            placeholder="Название, alias или логин"
            aria-label="Поиск учётных данных"
            @input="search"
          /><button type="button" @click="newEntry()">Добавить</button>
        </div>
        <form
          v-if="editing"
          class="vault-editor"
          autocomplete="off"
          @submit.prevent="save(false)"
        >
          <h3>{{ editing.id ? 'Изменить запись' : 'Новая запись' }}</h3>
          <label
            >Название<input v-model="editing.title" required maxlength="160"
          /></label>
          <label
            >Alias<input
              v-model="editing.alias"
              required
              maxlength="120"
              autocomplete="off"
          /></label>
          <label
            >Сайт-подсказка<input
              v-model="editing.site_hint"
              type="url"
              placeholder="https://example.com"
          /></label>
          <label
            >Метка аккаунта<input
              v-model="editing.account_label"
              placeholder="Личный / рабочий"
          /></label>
          <label
            >Логин<input
              v-model="editing.username"
              :required="!editing.id"
              autocomplete="off"
              :placeholder="
                editing.id ? 'Оставьте пустым, чтобы сохранить' : ''
              "
          /></label>
          <label
            >Пароль<input
              v-model="editing.password"
              type="password"
              :required="!editing.id"
              autocomplete="new-password"
              :placeholder="
                editing.id ? 'Оставьте пустым, чтобы сохранить' : ''
              "
          /></label>
          <button class="vault-primary" type="submit" :disabled="busy">
            Сохранить
          </button>
          <button
            v-if="creatingFor"
            type="button"
            :disabled="busy"
            @click="save(true)"
          >
            Сохранить и разрешить для этого входа
          </button>
          <button type="button" @click="clearEditor">Отмена</button>
        </form>
        <form v-if="smsRoute" class="vault-editor" @submit.prevent="saveSMS">
          <h3>SMS-коды входа</h3>
          <label
            ><input v-model="smsRoute.enabled" type="checkbox" />Автоматически
            передавать код для этого аккаунта</label
          >
          <label
            >Телефон<select
              aria-label="Телефон"
              v-model="smsRoute.device_id"
              :required="smsRoute.enabled"
            >
              <option value="">Выберите подключённый телефон</option>
              <option
                v-for="device in smsDevices"
                :key="device.device_id"
                :value="device.device_id"
              >
                {{ device.name }}
              </option>
            </select></label
          >
          <label
            >Приложение SMS<input
              v-model="smsRoute.package_name"
              :required="smsRoute.enabled"
              placeholder="com.google.android.apps.messaging"
          /></label>
          <label
            >Точный отправитель SMS<input
              v-model="smsRoute.sender"
              :required="smsRoute.enabled"
              placeholder="Название или номер отправителя"
          /></label>
          <p class="vault-muted">
            Включите передачу SMS-кодов входа в компаньоне. Разрешение действует
            только для активного входа после вашего согласия на использование
            пароля. Платежи и неоднозначные сообщения требуют ручного ввода.
          </p>
          <button class="vault-primary" type="submit">Сохранить привязку</button
          ><button type="button" @click="smsRoute = null">Отмена</button>
        </form>
        <ul class="vault-entries">
          <li v-for="entry in entries" :key="entry.id">
            <div>
              <strong>{{ entry.title }}</strong
              ><small>{{ entry.alias }} · {{ entry.account_label }}</small
              ><small>{{ entry.site_hint || 'Сайт не указан' }}</small>
            </div>
            <button type="button" @click="editEntry(entry)">Изменить</button
            ><button type="button" @click="openSMS(entry)">SMS-коды</button
            ><button type="button" @click="remove(entry, false)">
              Отозвать</button
            ><button type="button" @click="remove(entry, true)">Удалить</button>
          </li>
        </ul>
        <p v-if="!entries.length && !editing" class="vault-muted">
          Записей пока нет. Добавьте первый аккаунт.
        </p>
      </aside>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import type { ComponentPublicInstance } from 'vue'
interface Entry {
  id: string
  alias: string
  title: string
  site_hint: string
  account_label: string
  revision: number
  status: string
}
interface Draft {
  id: string
  alias: string
  title: string
  site_hint: string
  account_label: string
  expected_revision: number
  username: string
  password: string
}
interface Request {
  login_scope?: { target_url: string }
  id: string
  kind: string
  target_origin: string
  purpose: string
  fields: string[]
  record_id: string
  record_label?: string
}
interface SMSRoute {
  record_id: string
  device_id: string
  package_name: string
  sender: string
  enabled: boolean
}
const props = defineProps<{ ownerToken: string }>()
const base = String(useRuntimeConfig().public.apiBase || '').replace(/\/$/, '')
const panelOpen = ref(false),
  pinOpen = ref(false),
  configured = ref(true),
  confirmation = ref(false),
  busy = ref(false),
  error = ref(''),
  digits = ref(Array<string>(6).fill(''))
const changingPin = ref(false)
const smsRoute = ref<SMSRoute | null>(null),
  smsDevices = ref<{ device_id: string; name: string }[]>([])
const entries = ref<Entry[]>([]),
  requests = ref<Request[]>([]),
  editing = ref<Draft | null>(null),
  creatingFor = ref(''),
  query = ref(''),
  otp = ref<Record<string, string>>({}),
  selectedRecords = ref<Record<string, string>>({})
let unlockToken = '',
  firstPin = '',
  timer: ReturnType<typeof setTimeout> | undefined,
  searchTimer: ReturnType<typeof setTimeout> | undefined
let pollTimer: ReturnType<typeof setInterval> | undefined
let vaultEpoch = 0
let requestedID =
  typeof window === 'undefined'
    ? ''
    : /^#credential=(creq_[a-f0-9]{64})$/.exec(window.location.hash)?.[1] || ''
const cells: HTMLInputElement[] = []
function setCell(el: Element | ComponentPublicInstance | null, index: number) {
  if (el instanceof HTMLInputElement) cells[index] = el
}
async function request<T>(body?: unknown): Promise<T> {
  const stamp = vaultEpoch,
    owner = props.ownerToken
  const response = await fetch(`${base}/api/v4/credentials`, {
    method: body === undefined ? 'GET' : 'POST',
    cache: 'no-store',
    credentials: 'omit',
    signal: AbortSignal.timeout(15000),
    headers: {
      Authorization: `Bearer ${props.ownerToken}`,
      'Content-Type': 'application/json',
      ...(unlockToken ? { 'X-Butler-Vault': unlockToken } : {}),
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  })
  if (stamp !== vaultEpoch || owner !== props.ownerToken)
    throw new Error('Операция отменена.')
  if (response.status === 423) {
    clearSecrets()
    throw new Error('Хранилище заблокировано. Откройте его снова.')
  }
  if (!response.ok)
    throw new Error(
      response.status === 503
        ? 'Хранилище ещё не настроено на сервере.'
        : 'Операция отклонена. Проверьте данные и повторите.',
    )
  if (unlockToken && body !== undefined) {
    clearTimeout(timer)
    timer = setTimeout(clearSecrets, 5 * 60 * 1000)
  }
  const data = (await response.json()) as T
  if (stamp !== vaultEpoch || owner !== props.ownerToken)
    throw new Error('Операция отменена.')
  return data
}
function clearEditor() {
  if (editing.value) {
    editing.value.username = ''
    editing.value.password = ''
  }
  editing.value = null
  creatingFor.value = ''
}
function clearSecrets() {
  vaultEpoch++
  clearTimeout(timer)
  clearTimeout(searchTimer)
  clearEditor()
  smsRoute.value = null
  smsDevices.value = []
  unlockToken = ''
  firstPin = ''
  changingPin.value = false
  otp.value = {}
  digits.value = Array<string>(6).fill('')
  panelOpen.value = false
  pinOpen.value = false
  entries.value = []
  requests.value = []
  query.value = ''
  selectedRecords.value = {}
}
async function beginPinChange() {
  clearEditor()
  panelOpen.value = false
  changingPin.value = true
  confirmation.value = false
  pinOpen.value = true
  await nextTick()
  cells[0]?.focus()
}
function close() {
  const token = unlockToken
  if (token) void request({ action: 'lock' }).catch(() => {})
  clearSecrets()
}
function lock() {
  close()
}
async function openVault() {
  error.value = ''
  try {
    configured.value = (await request<{ configured: boolean }>()).configured
    confirmation.value = false
    pinOpen.value = true
    await nextTick()
    cells[0]?.focus()
  } catch (e) {
    error.value = (e as Error).message
    pinOpen.value = true
  }
}
function inputDigit(event: Event, index: number) {
  const input = event.target as HTMLInputElement
  const digit = input.value.replace(/\D/g, '').slice(-1)
  input.value = digit
  digits.value[index] = digit
  if (digits.value[index]) cells[index + 1]?.focus()
}
function trapFocus(event: KeyboardEvent) {
  const focusable = [
    ...(event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>(
      'button:not(:disabled), input:not(:disabled), select:not(:disabled), [tabindex="0"]',
    ),
  ].filter((el) => el.getClientRects().length > 0)
  const first = focusable[0],
    last = focusable.at(-1)
  if (event.shiftKey && event.target === first) {
    event.preventDefault()
    last?.focus()
  } else if (!event.shiftKey && event.target === last) {
    event.preventDefault()
    first?.focus()
  }
}
function keyDigit(event: KeyboardEvent, index: number) {
  if (event.key === 'Backspace' && !digits.value[index] && index > 0) {
    digits.value[index - 1] = ''
    cells[index - 1]?.focus()
  }
  if (event.key === 'ArrowLeft') cells[index - 1]?.focus()
  if (event.key === 'ArrowRight') cells[index + 1]?.focus()
}
function pastePin(event: ClipboardEvent) {
  const value = event.clipboardData?.getData('text').replace(/\D/g, '') || ''
  if (value.length === 6) {
    digits.value = value.split('')
    cells[5]?.focus()
  }
}
async function submitPin() {
  if (busy.value || digits.value.some((d) => !d)) return
  error.value = ''
  const pin = digits.value.join('')
  digits.value = Array<string>(6).fill('')
  if ((!configured.value || changingPin.value) && !confirmation.value) {
    firstPin = pin
    confirmation.value = true
    await nextTick()
    cells[0]?.focus()
    return
  }
  busy.value = true
  try {
    if (changingPin.value) {
      await request({ action: 'change_pin', pin, confirm: firstPin })
      clearSecrets()
      return
    }
    unlockToken = (
      await request<{ unlock_token: string }>({
        action: 'unlock',
        pin,
        confirm: firstPin,
      })
    ).unlock_token
    firstPin = ''
    pinOpen.value = false
    panelOpen.value = true
    await refresh()
  } catch {
    error.value = 'PIN не принят. Проверьте цифры или повторите позже.'
    firstPin = ''
    confirmation.value = false
    await nextTick()
    cells[0]?.focus()
  } finally {
    busy.value = false
  }
}
async function refresh() {
  const data = await request<{ entries: Entry[] }>({
    action: 'list',
    query: query.value,
  })
  entries.value = data.entries
  const pending = await request<{ requests: Request[] }>({ action: 'requests' })
  requests.value = pending.requests
  for (const r of pending.requests)
    if (!selectedRecords.value[r.id]) selectedRecords.value[r.id] = r.record_id
}
function search() {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    void refresh().catch((e) => {
      error.value = e.message
    })
  }, 250)
}
function newEntry(r?: Request) {
  clearEditor()
  smsRoute.value = null
  creatingFor.value = r?.id || ''
  editing.value = {
    id: '',
    alias: '',
    title: '',
    site_hint: r?.target_origin || '',
    account_label: '',
    expected_revision: 0,
    username: '',
    password: '',
  }
}
function editEntry(e: Entry) {
  clearEditor()
  smsRoute.value = null
  editing.value = {
    id: e.id,
    alias: e.alias,
    title: e.title,
    site_hint: e.site_hint,
    account_label: e.account_label,
    expected_revision: e.revision,
    username: '',
    password: '',
  }
}
async function save(allow: boolean) {
  if (!editing.value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    const pendingID = creatingFor.value
    const data = await request<{ entry: Entry }>({
      action: 'save',
      entry: editing.value,
    })
    clearEditor()
    if (allow && pendingID)
      await request({
        action: 'respond',
        id: pendingID,
        record_id: data.entry.id,
        allow: true,
      })
    await refresh()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
async function remove(e: Entry, erase: boolean) {
  if (erase && !window.confirm(`Удалить запись «${e.title}»?`)) return
  try {
    await request({
      action: erase ? 'delete' : 'revoke',
      id: e.id,
      revision: e.revision,
    })
    await refresh()
  } catch (e) {
    error.value = (e as Error).message
  }
}
async function respond(r: Request, allow: boolean) {
  try {
    const value = allow && r.kind === 'otp' ? otp.value[r.id] || '' : ''
    delete otp.value[r.id]
    await request({
      action: 'respond',
      id: r.id,
      record_id: r.kind === 'otp' ? '' : selectedRecords.value[r.id] || '',
      allow,
      otp: value,
    })
    await refresh()
  } catch (e) {
    error.value = (e as Error).message
  }
}
async function openSMS(entry: Entry) {
  try {
    clearEditor()
    const data = await request<{
      routes: SMSRoute[]
      devices: { device_id: string; name: string }[]
    }>({ action: 'sms_routes' })
    smsDevices.value = data.devices
    smsRoute.value = data.routes.find((r) => r.record_id === entry.id) || {
      record_id: entry.id,
      device_id: '',
      package_name: 'com.google.android.apps.messaging',
      sender: '',
      enabled: false,
    }
  } catch (e) {
    error.value = (e as Error).message
  }
}
async function saveSMS() {
  if (!smsRoute.value) return
  try {
    await request({ action: 'sms_route', sms_route: smsRoute.value })
    smsRoute.value = null
  } catch (e) {
    error.value = (e as Error).message
  }
}
async function poll() {
  const token = props.ownerToken
  if (!token || busy.value) return
  try {
    const data = await request<{ configured: boolean; requests: Request[] }>()
    if (token === props.ownerToken) {
      configured.value = data.configured === true
      requests.value = Array.isArray(data.requests) ? data.requests : []
      if (requestedID && requests.value.some((r) => r.id === requestedID)) {
        requestedID = ''
        window.history.replaceState(
          null,
          '',
          window.location.pathname + window.location.search,
        )
        await openVault()
      }
    }
  } catch {
    /* Server readiness is shown when the owner opens the vault. */
  }
}
watch(
  () => props.ownerToken,
  () => {
    clearSecrets()
    clearInterval(pollTimer)
    if (props.ownerToken) {
      void poll()
      pollTimer = setInterval(() => {
        void poll()
      }, 10000)
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  clearInterval(pollTimer)
  clearSecrets()
})
</script>

<style scoped>
.vault-bookmark {
  position: fixed;
  right: 0;
  top: 50%;
  z-index: 70;
  width: 42px;
  height: 62px;
  padding: 10px;
  border: 1px solid var(--color-border-strong);
  border-right: 0;
  border-radius: 12px 0 0 12px;
  background: var(--color-bg-canvas);
  color: var(--color-text-primary);
  cursor: pointer;
  transform: translateY(-50%);
}
.vault-bookmark svg {
  width: 100%;
  height: 100%;
  fill: currentcolor;
}
.vault-badge {
  position: absolute;
  top: -6px;
  left: -6px;
  padding: 2px 6px;
  border-radius: 20px;
  background: var(--color-accent-primary);
  color: var(--color-text-inverse);
  font-size: 11px;
}
.vault-overlay {
  position: fixed;
  inset: 0;
  z-index: 110;
  display: grid;
  place-items: center;
  padding: 20px;
  background: var(--color-bg-overlay);
  backdrop-filter: blur(5px);
}
.vault-pin-dialog {
  position: relative;
  width: min(430px, 100%);
  padding: 44px 28px 32px;
  border: 1px solid var(--color-border-default);
  border-radius: 22px;
  background: var(--color-bg-surface);
  text-align: center;
  box-shadow: var(--shadow-xl);
}
.vault-pin-dialog h2 {
  margin: 0 0 8px;
  font-size: 24px;
}
.vault-pin-dialog p {
  color: var(--color-text-secondary);
}
.vault-pin-cells {
  display: flex;
  justify-content: center;
  gap: 9px;
  margin: 28px 0;
}
.vault-pin-cells input {
  width: 43px;
  height: 51px;
  padding: 0;
  border: 1px solid var(--color-border-strong);
  border-radius: 9px;
  background: var(--color-bg-elevated);
  text-align: center;
  font-size: 25px;
}
.vault-close {
  position: absolute;
  top: 12px;
  right: 14px;
  border: 0;
  background: transparent;
  font-size: 27px;
}
.vault-primary {
  padding: 11px 20px;
  border: 0;
  border-radius: 9px;
  background: var(--color-accent-primary);
  color: var(--color-text-inverse);
}
.vault-error {
  color: var(--color-state-error) !important;
}
.vault-panel-overlay {
  justify-items: end;
  padding: 0;
}
.vault-panel {
  width: min(520px, 100%);
  height: 100%;
  overflow-y: auto;
  padding: 28px;
  border-left: 1px solid var(--color-border-default);
  background: var(--color-bg-surface);
  color: var(--color-text-primary);
  box-shadow: var(--shadow-xl);
}
.vault-panel header {
  display: flex;
  align-items: center;
  gap: 10px;
}
.vault-panel h2 {
  flex: 1;
  margin: 0;
  font-size: 23px;
}
.vault-panel button {
  cursor: pointer;
}
.vault-panel button:not(.vault-primary) {
  padding: 7px 11px;
  border: 1px solid var(--color-border-default);
  border-radius: 8px;
  background: var(--color-bg-elevated);
  color: var(--color-text-primary);
}
.vault-panel button:disabled {
  opacity: 0.45;
  cursor: default;
}
.vault-panel input,
.vault-panel select {
  width: 100%;
  min-height: 40px;
  padding: 8px 10px;
  border: 1px solid var(--color-border-strong);
  border-radius: 7px;
  background: var(--color-bg-elevated);
}
.vault-muted {
  color: var(--color-text-secondary);
  font-size: 13px;
}
.vault-panel input[type='checkbox'] {
  width: 18px;
  min-height: 18px;
  padding: 0;
}
.vault-search {
  display: flex;
  gap: 10px;
  margin: 24px 0;
}
.vault-editor,
.vault-request {
  display: grid;
  gap: 12px;
  margin: 20px 0;
  padding: 18px;
  border: 1px solid var(--color-border-default);
  border-radius: 12px;
}
.vault-editor label {
  display: grid;
  gap: 5px;
  font-size: 13px;
}
.vault-entries {
  padding: 0;
  list-style: none;
}
.vault-entries li {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 18px 0;
  border-bottom: 1px solid var(--color-border-default);
}
.vault-entries li div {
  width: 100%;
}
.vault-entries small {
  display: block;
  margin-top: 4px;
  color: var(--color-text-secondary);
}
.vault-prompt {
  position: fixed;
  top: 24px;
  right: 24px;
  z-index: 80;
  width: min(350px, calc(100% - 48px));
  padding: 20px;
  border: 1px solid var(--color-border-strong);
  border-radius: 16px;
  background: var(--color-bg-surface);
  color: var(--color-text-primary);
  box-shadow: var(--shadow-xl);
}
@media (width <= 420px) {
  .vault-pin-cells {
    gap: 5px;
  }
  .vault-pin-cells input {
    width: 35px;
    height: 43px;
  }
  .vault-panel {
    padding: 18px;
  }
}
</style>
