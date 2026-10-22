<template>
  <section class="mind-widget" :aria-label="question" :aria-busy="busy">
    <h3>{{ question }}</h3>
    <template v-if="view && input">
      <p role="status">{{ stateLabel }}</p>
      <fieldset :disabled="disabled">
        <legend class="mind-muted">{{ input.type === 'multi_choice' ? 'Выберите варианты, затем отправьте ответ' : input.type === 'text' ? 'Ваш ответ' : 'Выберите один вариант' }}</legend>
        <template v-for="option in input.options || []" :key="option.id">
          <label class="mind-widget-option">
            <input v-if="input.type === 'multi_choice'" type="checkbox" :checked="view.draft.option_ids?.includes(option.id)" @change="toggle(option.id)" />
            <input v-else type="radio" :name="view.interaction_ref" :checked="selected === option.id" @change="selected = option.id" />
            <span>{{ option.label }}<small v-if="option.description">{{ option.description }}</small></span>
          </label>
          <MindPublicationMedia v-if="option.image_ref && file(option.image_ref)" :publication="publication.publication_ref" :file="file(option.image_ref)!" :alt="option.label" image />
        </template>
        <label v-if="input.type === 'text' || input.allow_text_answer" class="mind-widget-text">
          {{ input.type === 'text' ? 'Ответ на вопрос' : 'Или свой вариант' }}
          <textarea v-model="text" :placeholder="input.placeholder" rows="3" @input="selected = ''" />
        </label>
        <button class="mind-text-button" type="button" :disabled="!valid" @click="submit">Отправить ответ</button>
      </fieldset>
      <p v-if="view.state === 'resolved'" class="mind-readable">{{ answerLabel }}</p>
      <p v-if="error" role="alert">{{ error }}</p>
      <button v-if="pending && !busy" class="mind-text-button" type="button" @click="sendPending">Повторить тот же ответ</button>
    </template>
    <p v-else class="mind-muted">Вопрос сейчас недоступен.</p>
  </section>
</template>
<script setup lang="ts">
import type { Publication, WidgetAnswer } from '../model/communications'
import { useMind } from '../model/store'
import MindPublicationMedia from './MindPublicationMedia.vue'
const props = defineProps<{ publication: Publication }>()
const store = useMind(), selected = ref(''), text = ref(''), busy = ref(false), error = ref('')
const view = computed(() => props.publication.interaction), input = computed(() => props.publication.payload?.input)
const question = computed(() => props.publication.payload?.question || 'Вопрос Батлера')
const file = (id: string) => props.publication.files.find(f => f.artifact_ref === id)
const pending = ref<{ delivery: string; body: unknown; path: string } | null>(null)
const disabled = computed(() => view.value?.state !== 'open' || busy.value || !!pending.value)
const labels: Record<string, string> = { open: 'Жду вашего ответа.', resolved: 'Ответ принят. Вопрос закрыт.', expired: 'Срок ответа истёк.', cancelled: 'Вопрос отменён.', superseded: 'Вопрос заменён новым уточнением.' }
const stateLabel = computed(() => labels[view.value?.state || ''] || 'Состояние вопроса проверяется.')
const answerLabel = computed(() => view.value?.answer.text || (view.value?.answer.option_ids || []).map(id => input.value?.options?.find(o => o.id === id)?.label || id).join(', '))
const valid = computed(() => {
  const q = input.value
  if (!q) return false
  if (q.type === 'multi_choice') { const n = view.value?.draft.option_ids?.length || 0; return n >= (q.min_selections || 0) && n <= (q.max_selections || q.options?.length || 0) }
  if (text.value.trim() && (q.type === 'text' || q.allow_text_answer)) return [...text.value].length >= (q.min_length || 0) && [...text.value].length <= (q.max_length || 4000)
  return q.type === 'single_choice' && !!selected.value
})
watch(() => store.epoch, () => { pending.value = null; text.value = ''; selected.value = ''; error.value = '' }, { flush: 'sync' })
function action(operation: string, answer: WidgetAnswer) {
  if (disabled.value || !view.value || !store.communicationProfile) return
  pending.value = { delivery: crypto.randomUUID(), path: `/api/v3/interactions/${encodeURIComponent(view.value.interaction_ref)}/actions`, body: {
    binding_id: store.communicationProfile.binding_id, binding_version: store.communicationProfile.binding_version,
    expected_version: view.value.version, draft_version: view.value.draft_version, operation, answer,
  } }
  void sendPending()
}
function toggle(id: string) {
  const ids = [...view.value?.draft.option_ids || []], index = ids.indexOf(id)
  if (index < 0) ids.push(id); else ids.splice(index, 1)
  action('draft', { option_ids: ids })
}
function submit() { action(input.value?.type === 'multi_choice' ? 'submit' : 'answer', input.value?.type === 'multi_choice' ? {} : text.value.trim() ? { text: text.value } : { option_ids: [selected.value] }) }
async function sendPending() {
  const request = pending.value, stamp = store.epoch
  if (!request || busy.value) return
  busy.value = true; error.value = ''
  try {
    await store.request(request.path, request.body, { delivery: request.delivery })
    if (stamp !== store.epoch) return
    pending.value = null
    await store.loadPublications()
  } catch (e) {
    if (stamp !== store.epoch) return
    if (e instanceof Error && ['version_changed', 'owner_denied', 'invalid_answer'].includes(e.message)) {
      pending.value = null
      error.value = e.message === 'version_changed' ? 'Вопрос уже закрыт или выбор изменён в другом канале. Проверьте актуальное состояние.' : e.message === 'invalid_answer' ? 'Проверьте допустимое число вариантов и длину ответа.' : 'Доступ к вопросу изменился.'
      await store.loadPublications()
    } else error.value = 'Ответ не подтверждён. Можно повторить тот же запрос.'
  } finally { if (stamp === store.epoch) busy.value = false }
}
</script>
<style scoped>
.mind-widget { padding-block: 8px; }
.mind-widget fieldset { min-width: 0; margin: 0; padding: 0; border: 0; }
.mind-widget-option { display: flex; gap: 10px; align-items: flex-start; padding-block: 10px; cursor: pointer; }
.mind-widget-option input { margin-top: 4px; }
.mind-widget-option small { display: block; white-space: pre-wrap; color: var(--color-text-secondary); }
.mind-widget-text { display: grid; gap: 8px; margin-block: 12px; }
.mind-widget textarea { width: 100%; padding: 10px; border: 1px solid var(--color-border-subtle); border-radius: 8px; color: inherit; background: transparent; font: inherit; }
</style>
