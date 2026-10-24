<template>
  <details class="mind-communication-diagnostics" @toggle="toggle">
    <summary>Состояние общения</summary>
    <button class="mind-text-button" type="button" :disabled="loading" @click="load">Обновить состояние</button>
    <p v-if="error" class="mind-error" role="alert">{{ error }}</p>
    <template v-if="report">
      <p v-if="report.drain_only" class="mind-muted">Приём новых сообщений приостановлен. Сохранённые вопросы и результаты остаются доступны.</p>
      <p v-if="!report.full_v1_qualified" class="mind-muted">Полная приёмка общения ещё не подтверждена.</p>
      <p class="mind-muted">Доступность в вебе и подтверждение канала учитываются отдельно. Подтверждение канала не означает прочтение или выполнение поручения.</p>
      <h3>Доставка</h3>
      <p v-if="!report.deliveries.length" class="mind-muted">Сохранённых публикаций пока нет.</p>
      <p v-for="row in report.deliveries" :key="`${row.channel}-${row.state}`">
        {{ row.channel === 'web' ? 'Веб' : 'Telegram' }} · {{ state(row.state) }} · {{ row.count }}
      </p>
      <h3>Ответы и вопросы</h3>
      <p v-if="report.open_question_limit" class="mind-muted">Одновременно открытых вопросов: не более {{ report.open_question_limit }}.</p>
      <p v-for="row in report.obligations" :key="row.state">{{ state(row.state) }} · {{ row.count }}</p>
      <p v-for="row in report.interactions" :key="`question-${row.state}`">Вопрос: {{ state(row.state) }} · {{ row.count }}</p>
      <h3>Подключённые каналы</h3>
      <p v-for="binding in report.bindings" :key="binding.binding_id">
        {{ binding.channel === 'web' ? 'Веб' : 'Telegram' }} · {{ binding.current ? 'подключён' : binding.enabled ? 'нужна проверка подключения' : 'отключён' }}
      </p>
      <details><summary>Проверки и сохранённые попытки</summary>
        <p v-for="check in report.checks" :key="check.name">{{ checks[check.name] || check.name }} · {{ state(check.status) }}</p>
        <p v-for="input in report.inputs" :key="input.input_ref"><code>{{ input.input_ref }}</code> · {{ state(input.state) }} · {{ state(input.obligation) }}</p>
        <p v-if="report.inputs_truncated" class="mind-muted">Показаны последние 100 попыток; итоговые счётчики включают всю историю.</p>
      </details>
      <details v-if="report.legacy_deliveries.length"><summary>Прежние результаты</summary>
        <p v-for="row in report.legacy_deliveries" :key="`${row.channel}-${row.state}`">{{ row.channel === 'web' ? 'Веб' : 'Telegram' }} · {{ state(row.state) }} · {{ row.count }}</p>
      </details>
    </template>
  </details>
</template>
<script setup lang="ts">
import { useMind } from '../model/store'
const store = useMind()
interface Count { channel?: string; state: string; count: number }
interface Report {
  drain_only: boolean; full_v1_qualified: boolean; open_question_limit?: number
  deliveries: Count[]; obligations: Count[]; interactions: Count[]; legacy_deliveries: Count[]
  bindings: { binding_id: string; channel: string; enabled: boolean; current: boolean }[]
  checks: { name: string; status: string }[]
  inputs: { input_ref: string; state: string; obligation: string }[]
  inputs_truncated: boolean
}
const report = ref<Report | null>(null), loading = ref(false), error = ref('')
let request: AbortController | undefined
const states: Record<string, string> = {
  result_ready: 'доступно в вебе', pending: 'ожидает отправки', sending: 'отправляется', sent: 'канал принял',
  delivery_unknown: 'результат доставки неизвестен', failed: 'ошибка', open: 'ожидается ответ', resolved: 'ответ сохранён',
  waiting_for_user: 'ожидается ответ владельца', response_committed: 'ответ опубликован', closed_with_failure: 'ответ не завершён из-за ошибки',
  cancelled: 'отменено', expired: 'срок истёк', superseded: 'условия изменились', healthy: 'проверено', degraded: 'нужна проверка',
  unhealthy: 'не готово', unknown: 'не проверено', queued: 'в очереди', running: 'выполняется', blocked: 'выполнение остановлено',
}
const checks: Record<string, string> = { schema: 'Хранилище', source_policy: 'Проверка доступа', owner_question_limit: 'Лимит открытых вопросов', worker_epoch: 'Совместимость исполнителя', web: 'Веб-интерфейс', adapter_profile: 'Каналы',
  'communication.message': 'Сообщения', 'communication.photo': 'Фотографии', 'communication.report': 'Отчёты', 'communication.widget': 'Вопросы', 'binding.web': 'Подключение веба', 'binding.telegram': 'Подключение Telegram' }
const state = (value: string) => states[value] || 'не проверено'
function clear() { request?.abort(); report.value = null; error.value = ''; loading.value = false }
watch(() => store.communicationProfile, clear)
onUnmounted(clear)
async function load() {
  const profile = store.communicationProfile
  if (!profile || loading.value) return
  request?.abort()
  const owned = new AbortController(); request = owned
  loading.value = true; report.value = null; error.value = ''
  try {
    const response = await store.privateRead('/api/v3/communications/diagnostics', owned.signal)
    if (!response.ok) throw new Error('observation_unavailable')
    const value = await response.json() as Report
    if (profile === store.communicationProfile && !owned.signal.aborted) report.value = value
  } catch {
    if (profile === store.communicationProfile && !owned.signal.aborted) error.value = 'Состояние общения сейчас недоступно. Результат проверки неизвестен.'
  } finally { if (request === owned) loading.value = false }
}
function toggle(event: Event) { if ((event.target as HTMLDetailsElement).open && !report.value) void load() }
</script>
<style scoped>
.mind-communication-diagnostics { margin-block: 16px; }
.mind-communication-diagnostics h3 { margin-top: 12px; font-size: 16px; }
.mind-communication-diagnostics code { overflow-wrap: anywhere; }
</style>
