<template>
  <div v-if="store.panel === 'memory' && !store.inspection" class="mind-muted">
    {{ store.memoryError || 'Открываю источник…' }}
  </div>
  <div
    v-else-if="
      store.inspection &&
      (store.inspection.source.status !== 'active' ||
        store.inspection.source.origin?.sensitivity === 'secret')
    "
    class="mind-muted"
  >
    Этот источник закрыт для редактирования.<button
      class="mind-text-button"
      type="button"
      @click="openAdmin"
    >
      Версии и доступ ↗
    </button>
  </div>
  <form
    v-else
    class="mind-form"
    @submit.prevent="store.saveMemory(title, body)"
  >
    <label
      >Название<input
        v-model="title"
        aria-label="Название заметки"
        maxlength="200"
        :readonly="!!store.pendingNote && !store.inspection"
        required /></label
    ><label
      >Текст<textarea
        v-model="body"
        aria-label="Текст заметки"
        rows="7"
        maxlength="64000"
        :readonly="!!store.pendingNote && !store.inspection"
        required
      />
    </label>
    <p v-if="store.inspection" class="mind-source-caption">
      {{ memoryOrigin(store.inspection.source) }} ·
      {{ dateLabel(store.inspection.source.extracted_at) }} · версия
      {{ store.inspection.source.revision }}
    </p>
    <p v-if="store.memoryError" role="alert">{{ store.memoryError }}</p>
    <div class="mind-actions">
      <button
        v-if="store.inspection"
        class="mind-text-button"
        type="button"
        :disabled="store.busy"
        @click="removeConfirm = !removeConfirm"
      >
        Удалить</button
      ><button class="mind-button primary" :disabled="store.busy">
        {{
          store.pendingNote && !store.inspection
            ? 'Повторить сохранение'
            : 'Сохранить'
        }}
      </button>
    </div>
    <div v-if="removeConfirm" class="mind-delete-confirm">
      <p>Удалить этот источник и его производные из памяти Butler?</p>
      <button
        class="mind-button"
        type="button"
        :disabled="store.busy"
        @click="store.hideMemory('delete')"
      >
        Удалить источник и производные
      </button>
    </div>
    <details v-if="store.inspection?.fragments?.length">
      <summary>Фрагменты источника</summary>
      <p
        v-for="fragment in store.inspection.fragments"
        :key="fragment.chunk_id"
      >
        {{ fragment.text }}
      </p>
    </details>
  </form>
</template>
<script setup lang="ts">
import { useMind } from '../model/store'
import { dateLabel, memoryOrigin } from '../model/presentation'
function openAdmin() {
  store.advanced = 'memory'
  store.panel = 'advanced'
}
const store = useMind(),
  title = ref(''),
  body = ref(''),
  removeConfirm = ref(false)
watch(
  () => store.inspection,
  (inspection) => {
    title.value = inspection
      ? inspection.source.title
      : store.panel === 'note'
        ? store.pendingNote?.title || ''
        : ''
    body.value = inspection
      ? inspection.source.body
      : store.panel === 'note'
        ? store.pendingNote?.body || ''
        : ''
    removeConfirm.value = false
  },
  { immediate: true },
)
</script>
