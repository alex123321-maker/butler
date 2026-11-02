<template>
  <section v-if="current" class="outcome-review" aria-live="polite" :key="current.review_id">
    <h2>Проверьте результат действия</h2>
    <p>{{ current.resource_hint }}</p>
    <p>Не удалось подтвердить результат отправки. Проверьте состояние у получателя. Ответ не повторяет отправку.</p>
    <label>
      Проверенный исход
      <select v-model="outcome" :disabled="busy">
        <option value="">Выберите исход</option>
        <option value="effect_confirmed">Операция выполнена</option>
        <option value="effect_absent">Операция не выполнена</option>
        <option value="unknown">Не удалось проверить</option>
      </select>
    </label>
    <template v-if="known">
      <label><input v-model="checked" type="checkbox" :disabled="busy"> Я проверил состояние у получателя</label>
      <label><input v-model="settled" type="checkbox" :disabled="busy"> Обработка завершена, отложенных операций нет</label>
      <label>Номер или ссылка на подтверждение, без паролей
        <input v-model="receipt" type="text" autocomplete="off" maxlength="2048" :disabled="busy">
      </label>
    </template>
    <p v-if="error" role="alert">{{ error }}</p>
    <button type="button" :disabled="!canSend" @click="submit">Сообщить исход</button>
    <small>Если исход неизвестен, поручение останется остановленным.</small>
  </section>
</template>

<script setup lang="ts">
interface Review {
  review_id: string
  effect_id: string
  fingerprint: string
  revision: number
  resource_hint: string
  expires_at: string
}
const props = defineProps<{ ownerToken: string }>()
const base = String(useRuntimeConfig().public.apiBase || '').replace(/\/$/, '')
const reviews = ref<Review[]>([])
const current = computed(() => reviews.value[0])
const outcome = ref(''), receipt = ref(''), error = ref('')
const checked = ref(false), settled = ref(false), busy = ref(false)
const known = computed(() => outcome.value === 'effect_confirmed' || outcome.value === 'effect_absent')
const canSend = computed(() => !busy.value && !!current.value && !!outcome.value && (!known.value || (checked.value && settled.value && !!receipt.value.trim())))
let epoch = 0, polling = false
let timer: ReturnType<typeof setInterval> | undefined
function clearDraft() {
  outcome.value = ''; receipt.value = ''; checked.value = false; settled.value = false; error.value = ''
}
async function request(body?: unknown) {
  const token = props.ownerToken, stamp = epoch
  const response = await fetch(`${base}/api/v4/action-outcomes`, {
    method: body === undefined ? 'GET' : 'POST', credentials: 'omit', cache: 'no-store',
    signal: AbortSignal.timeout(10000),
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json', 'X-Butler-Delivery-ID': crypto.randomUUID() },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  })
  if (!response.ok) throw new Error('outcome unavailable')
  const data = await response.json()
  if (stamp !== epoch || token !== props.ownerToken) throw new Error('owner changed')
  return data
}
async function refresh() {
  if (polling || busy.value || !props.ownerToken) return
  polling = true
  try {
    const data = await request()
    reviews.value = data.reviews || []
  } catch { /* Retain the existing decision card on a transient error. */ }
  finally { polling = false }
}
async function submit() {
  const review = current.value
  if (!review || !canSend.value) return
  const stamp = epoch
  busy.value = true; error.value = ''
  try {
    await request({ action: 'report', decision: {
      review_id: review.review_id, fingerprint: review.fingerprint, revision: review.revision,
      outcome: outcome.value, recipient_checked: known.value && checked.value, processing_settled: known.value && settled.value,
      receipt_reference: known.value ? receipt.value : '',
    } })
    reviews.value = reviews.value.filter(r => r.review_id !== review.review_id)
    clearDraft()
  } catch { if (stamp === epoch) error.value = 'Ответ не подтверждён. Проверьте состояние перед новым действием.' }
  finally { if (stamp === epoch) { busy.value = false; receipt.value = '' } }
}
watch(() => current.value?.review_id, clearDraft)
watch(() => props.ownerToken, () => {
  epoch++; reviews.value = []; busy.value = false; polling = false; clearDraft()
  if (timer) clearInterval(timer)
  if (props.ownerToken) { void refresh(); timer = setInterval(() => { void refresh() }, 2500) }
}, { immediate: true })
onBeforeUnmount(() => { epoch++; if (timer) clearInterval(timer); reviews.value = []; clearDraft() })
</script>

<style scoped>
.outcome-review { position: fixed; left: 1rem; bottom: 1rem; z-index: 80; max-width: min(25rem, calc(100vw - 2rem)); max-height: 80vh; overflow: auto; padding: 1rem; border: 1px solid var(--color-border-strong); border-radius: .8rem; background: var(--color-bg-surface); color: var(--color-text-primary); box-shadow: var(--shadow-xl); }
h2 { font-size: 1.1rem; margin: 0 0 .75rem; }
p { margin: .6rem 0; overflow-wrap: anywhere; }
label { display: block; margin: .7rem 0; }
select, input[type=text] { display: block; width: 100%; padding: .45rem; margin-top: .3rem; border: 1px solid var(--color-border-strong); border-radius: .3rem; background: var(--color-bg-elevated); color: var(--color-text-primary); }
button { padding: .55rem .8rem; border: 1px solid var(--color-border-strong); border-radius: .3rem; }
small { display: block; margin-top: .7rem; }
</style>
