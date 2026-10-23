<template>
  <section class="mind-memory" aria-label="Память">
    <div class="mind-memory-bar">
      <form
        role="search"
        :aria-busy="store.memoryBusy"
        @submit.prevent="searchNow"
      >
        <MindIcon name="stack" /><label class="sr-only" for="mind-memory-query"
          >Поиск в памяти</label
        ><input
          id="mind-memory-query"
          v-model="store.memoryQuery"
          type="search"
          maxlength="500"
          placeholder="Что вспомнить?"
          :disabled="!store.connected"
          @input="queueSearch"
        /><button
          v-if="store.memoryQuery"
          class="mind-icon-button"
          type="button"
          aria-label="Очистить поиск"
          @click="clear"
        >
          <MindIcon name="close" /></button
        ><button
          class="mind-icon-button"
          aria-label="Найти в памяти"
          :disabled="!store.connected"
        >
          <MindIcon name="search" />
        </button>
      </form>
      <button
        class="mind-icon-button"
        type="button"
        aria-label="Добавить заметку"
        :disabled="!store.connected"
        @click="addNote"
      >
        <MindIcon name="plus" />
      </button>
    </div>
    <div class="mind-memory-state">
      <span role="status">{{
        store.memoryBusy
          ? 'Ищу…'
          : store.memoryError ||
            (store.memoryQuery
              ? store.memoryMode === 'semantic'
                ? 'По смыслу'
                : 'По тексту'
              : 'Недавно добавлено')
      }}</span
      ><button
        v-if="store.memoryQuery && store.memoryMode !== 'semantic'"
        class="mind-help"
        type="button"
        title="Поиск работает по тексту. Семантический запрос доступен через агента с действующими полномочиями и бюджетом."
        aria-label="Режим поиска"
        @click="showMode = !showMode"
      >
        ?
      </button>
    </div>
    <p v-if="showMode" class="mind-muted">
      Здесь используется поиск по тексту. Для смыслового поиска можно попросить
      Батлера найти нужное в памяти.
    </p>
    <button class="mind-text-button" type="button" :disabled="!store.connected" @click="store.panel = 'memory'">Обзор памяти ↗</button>
    <TransitionGroup name="mind-entry" tag="div"
      ><button
        v-for="source in store.memories.slice(0, 3)"
        :key="source.source_id"
        class="mind-memory-result"
        type="button"
        @click="store.inspectMemory(source.source_id)"
      >
        <span
          class="mind-source-icon"
          :class="source.source_kind === 'model_candidate' ? 'sand' : 'sage'"
          :title="source.source_kind"
          ><MindIcon
            :name="source.redacted ? 'lock' : memoryIcon(source)" /></span
        ><span class="mind-memory-copy"
          ><span class="mind-memory-title">{{
            source.title || 'Без названия'
          }}</span
          ><span class="mind-memory-excerpt">{{
            source.redacted ? 'Закрытое содержимое' : source.body
          }}</span
          ><span class="mind-memory-meta"
            ><span>{{ memoryOrigin(source) }}</span
            ><time>{{
              dateLabel(source.created_at || source.extracted_at)
            }}</time></span
          ></span
        ><span class="mind-open-mark" aria-hidden="true">↗</span>
      </button></TransitionGroup
    >
    <p
      v-if="
        store.connected &&
        !store.memoryBusy &&
        !store.memoryError &&
        !store.memories.length
      "
      class="mind-muted"
    >
      {{
        store.memoryQuery ? 'Ничего похожего не найдено.' : 'Пока здесь пусто.'
      }}
    </p>
  </section>
</template>
<script setup lang="ts">
import { useMind } from '../model/store'
import { dateLabel, memoryIcon, memoryOrigin } from '../model/presentation'
import MindIcon from './MindIcon.vue'
const store = useMind(),
  showMode = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
function addNote() {
  store.clearInspection()
  store.panel = 'note'
}
function queueSearch() {
  clearTimeout(timer)
  store.memories = []
  if (!store.connected) return
  store.memoryBusy = true
  const epoch = store.epoch
  timer = setTimeout(() => {
    if (store.epoch === epoch && store.connected) void store.loadMemory()
  }, 240)
}
function searchNow() {
  clearTimeout(timer)
  void store.loadMemory()
}
function clear() {
  clearTimeout(timer)
  store.memoryQuery = ''
  void store.loadMemory('')
}
watch(
  () => store.epoch,
  () => {
    clearTimeout(timer)
    showMode.value = false
  },
)
onBeforeUnmount(() => clearTimeout(timer))
</script>
