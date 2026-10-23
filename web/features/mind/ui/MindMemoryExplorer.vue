<template>
  <section
    ref="root"
    class="memory-explorer"
    :class="{ 'memory-reading': reading }"
    aria-label="Просмотр памяти"
  >
    <form class="memory-search" role="search" @submit.prevent="search()">
      <MindIcon name="search" />
      <input
        ref="input"
        v-model="query"
        type="search"
        maxlength="500"
        aria-label="Поиск записей"
        placeholder="Что хотите вспомнить?"
        @input="queueSearch"
        @keydown.down.prevent="focusResult(0)"
      />
      <button
        v-if="query"
        class="mind-icon-button"
        type="button"
        aria-label="Очистить запрос"
        @click="clear"
      >
        <MindIcon name="close" />
      </button>
      <kbd>Ctrl K</kbd><button class="mind-button" type="submit">Найти</button>
    </form>
    <div
      v-if="!query && store.memoryRecentQueries.length"
      class="memory-recent"
      aria-label="Недавние запросы"
    >
      <span>Недавно искали</span
      ><button
        v-for="recent in store.memoryRecentQueries"
        :key="recent"
        class="mind-text-button"
        type="button"
        @click="repeatSearch(recent)"
      >
        {{ recent }}
      </button>
      <button
        class="mind-icon-button"
        type="button"
        aria-label="Очистить недавние запросы"
        @click="store.memoryRecentQueries = []"
      >
        <MindIcon name="close" />
      </button>
    </div>
    <div class="memory-explorer-bar">
      <span role="status">{{
        store.memoryBusy
          ? 'Ищу…'
          : query.trim()
            ? `Найдено: ${store.memories.length}${store.memoryHasMore ? '+' : ''}`
            : 'Недавние записи'
      }}</span>
      <button
        class="mind-text-button memory-view-toggle"
        type="button"
        :aria-pressed="graph"
        @click="graph = !graph"
      >
        {{ graph ? 'Вернуться к списку' : 'Схема связей' }}
      </button>
      <button
        class="mind-icon-button"
        type="button"
        aria-label="Обновить память"
        :disabled="store.memoryBusy"
        @click="refresh"
      >
        <MindIcon name="refresh" />
      </button>
    </div>
    <p v-if="store.memoryError" class="mind-error" role="alert">
      {{ store.memoryError }}
      <button class="mind-text-button" type="button" @click="search()">
        Повторить
      </button>
    </p>
    <p
      v-if="store.memorySearchHint"
      class="mind-muted memory-search-hint"
      role="status"
    >
      {{ store.memorySearchHint }}
    </p>
    <div class="memory-workspace">
      <div ref="results" class="memory-results" :aria-busy="store.memoryBusy">
        <div
          v-if="!graph"
          class="memory-list"
          aria-label="Записи памяти"
          @keydown="listKey"
        >
          <button
            v-for="source in store.memories"
            :key="source.source_id"
            class="memory-list-row"
            :class="{ selected: source.source_id === store.memorySourceId }"
            type="button"
            :aria-pressed="source.source_id === store.memorySourceId"
            @click="select(source.source_id, true)"
          >
            <span class="mind-source-icon sage"
              ><MindIcon :name="memoryIcon(source)"
            /></span>
            <span class="memory-list-copy"
              ><strong>{{ source.title || 'Без названия' }}</strong
              ><span>{{
                readableSource(source) ? source.body : 'Закрытое содержимое'
              }}</span
              ><small
                >{{ memoryOrigin(source)
                }}{{
                  source.match === 'semantic'
                    ? ' · По смыслу'
                    : source.match === 'both'
                      ? ' · Текст и смысл'
                      : ''
                }}</small
              ></span
            >
            >
            <time>{{
              dateLabel(source.created_at || source.extracted_at)
            }}</time>
          </button>
        </div>
        <MindMemoryGraph
          v-else
          :sources="store.memories"
          :selected="store.memorySourceId"
          @select="select($event, true)"
        />
        <div
          v-if="
            !store.memoryBusy && !store.memoryError && !store.memories.length
          "
          class="memory-empty"
        >
          <p>
            {{
              query.trim()
                ? 'По этим словам ничего не нашлось.'
                : 'Здесь появятся сохранённые записи.'
            }}
          </p>
          <template v-if="query.trim()"
            ><p>
              Попробуйте одно-два характерных слова из сообщения или документа.
            </p>
            <button class="mind-button" type="button" @click="clear">
              Показать недавние записи
            </button></template
          >
        </div>
        <button
          v-if="store.memoryHasMore && store.memories.length <= 1000"
          class="mind-text-button memory-more"
          type="button"
          :disabled="store.memoryBusy"
          @click="more"
        >
          Загрузить ещё
        </button>
        <p
          v-if="store.memoryHasMore && store.memories.length > 1000"
          class="mind-muted"
        >
          Уточните запрос, чтобы найти более ранние записи.
        </p>
      </div>
      <aside
        ref="inspector"
        class="memory-inspector"
        aria-label="Выбранная запись"
        :aria-busy="store.memoryDetailBusy"
      >
        <nav class="memory-reader-nav" aria-label="Навигация по записям">
          <button
            class="mind-text-button memory-back-list"
            type="button"
            @click="backToList"
          >
            ← К списку
          </button>
          <button
            class="mind-icon-button"
            type="button"
            aria-label="Предыдущая просмотренная запись"
            :disabled="historyIndex <= 0"
            @click="travel(-1)"
          >
            <MindIcon name="previous" />
          </button>
          <button
            class="mind-icon-button"
            type="button"
            aria-label="Следующая просмотренная запись"
            :disabled="historyIndex >= history.length - 1"
            @click="travel(1)"
          >
            <MindIcon name="next" />
          </button>
          <span
            v-if="
              store.inspection &&
              !store.memories.some((s) => s.source_id === store.memorySourceId)
            "
            class="memory-outside"
            >Вне текущего списка</span
          >
        </nav>
        <p v-if="store.memoryDetailBusy" class="memory-empty" role="status">
          Открываю запись…
        </p>
        <p
          v-else-if="store.memoryDetailError"
          class="memory-empty"
          role="alert"
        >
          {{ store.memoryDetailError }}
          <button class="mind-text-button" type="button" @click="reloadSource">
            Повторить
          </button>
        </p>
        <MindMemoryDetail
          v-else-if="store.inspection"
          :key="`${store.memorySourceId}:${store.inspection.source.revision}`"
          :inspection="store.inspection"
          :related="related"
          :claims="claims"
          :claims-busy="claimsBusy"
          :claims-error="claimsError"
          :deciding="deciding || claimsBusy || !!claimsError"
          @select="select($event, true)"
          @decide="decide"
          @manage="manage"
          @refresh="reloadSource"
          @graph="showGraph"
        />
        <p v-else class="memory-empty">Выберите запись, чтобы прочитать её.</p>
      </aside>
    </div>
    <footer class="memory-explorer-footer">
      <span
        >{{ store.memories.length }} записей{{
          store.memoryHasMore ? ' · есть ещё' : ''
        }}</span
      ><span>{{
        store.memoryMode === 'hybrid' ? 'Текст и смысл' : 'Поиск по тексту'
      }}</span>
    </footer>
  </section>
</template>
<script setup lang="ts">
import { useMind } from '../model/store'
import { useMemoryClaims } from '../model/useMemoryClaims'
import { readableSource, sourceParents } from '../model/memory-explorer'
import { dateLabel, memoryOrigin, memoryIcon } from '../model/presentation'
import MindIcon from './MindIcon.vue'
import MindMemoryDetail from './MindMemoryDetail.vue'
import MindMemoryGraph from './MindMemoryGraph.vue'
const store = useMind(),
  router = useRouter()
const query = ref(store.memoryQuery),
  graph = ref(false),
  reading = ref(!!store.memorySourceId)
const input = ref<HTMLInputElement | null>(null),
  root = ref<HTMLElement | null>(null),
  results = ref<HTMLElement | null>(null),
  inspector = ref<HTMLElement | null>(null)
const history = ref(store.memorySourceId ? [store.memorySourceId] : []),
  historyIndex = ref(history.value.length - 1)
const {
  claims,
  busy: claimsBusy,
  error: claimsError,
  deciding,
  decide,
} = useMemoryClaims()
let timer: ReturnType<typeof setTimeout> | undefined,
  alive = true
const related = computed(() => {
  const source = store.inspection?.source
  if (!source || !readableSource(source)) return []
  const result = sourceParents(source).map((id, i) => ({
    id,
    title:
      store.memories.find((s) => s.source_id === id)?.title ||
      `Исходная запись ${i + 1}`,
    reason: 'Источник этой записи',
  }))
  for (const s of store.memories)
    if (
      sourceParents(s).includes(source.source_id) &&
      !result.some((r) => r.id === s.source_id)
    )
      result.push({
        id: s.source_id,
        title: s.title || 'Без названия',
        reason: 'Создано на основе этой записи',
      })
  return result
})
const narrow = () => window.matchMedia('(max-width: 700px)').matches
async function select(id: string, open = false, remember = true) {
  if (remember && history.value[historyIndex.value] !== id) {
    history.value = [
      ...history.value.slice(0, historyIndex.value + 1),
      id,
    ].slice(-30)
    historyIndex.value = history.value.length - 1
  }
  if (open) reading.value = true
  await store.inspectMemory(id, false)
  if (!alive || store.memorySourceId !== id) return
  await nextTick()
  if (inspector.value) inspector.value.scrollTop = 0
  if (open && narrow())
    inspector.value?.querySelector<HTMLElement>('h3')?.focus()
}
function travel(step: number) {
  const next = historyIndex.value + step
  if (next < 0 || next >= history.value.length) return
  historyIndex.value = next
  void select(history.value[next]!, true, false)
}
function backToList() {
  reading.value = false
  nextTick(() => {
    const selected = root.value?.querySelector<HTMLButtonElement>(
      '.memory-list-row.selected',
    )
    if (selected) selected.focus({ preventScroll: true })
    else input.value?.focus()
  })
}
function showGraph() {
  graph.value = true
  reading.value = false
}
function focusResult(index: number) {
  const rows =
    root.value?.querySelectorAll<HTMLButtonElement>('.memory-list-row')
  if (!rows?.length) return
  const bounded = Math.max(0, Math.min(rows.length - 1, index))
  rows[bounded]?.focus()
}
function listKey(event: KeyboardEvent) {
  const rows = Array.from(
    root.value?.querySelectorAll<HTMLButtonElement>('.memory-list-row') || [],
  )
  const index = rows.indexOf(event.target as HTMLButtonElement)
  if (index < 0) return
  if (
    event.key === 'ArrowDown' ||
    event.key === 'ArrowUp' ||
    event.key === 'Home' ||
    event.key === 'End'
  ) {
    event.preventDefault()
    focusResult(
      event.key === 'Home'
        ? 0
        : event.key === 'End'
          ? rows.length - 1
          : index + (event.key === 'ArrowDown' ? 1 : -1),
    )
  }
}
function queueSearch() {
  clearTimeout(timer)
  store.cancelMemorySearch()
  store.memoryQuery = query.value
  store.memories = []
  store.memoryHasMore = false
  store.memoryBusy = true
  reading.value = false
  timer = setTimeout(() => search('text'), 240)
}
function search(strategy: 'text' | 'hybrid' = 'hybrid') {
  clearTimeout(timer)
  timer = undefined
  reading.value = false
  void store.loadMemory(query.value, false, strategy)
}
function clear() {
  query.value = ''
  search()
  input.value?.focus()
}
function repeatSearch(value: string) {
  query.value = value
  search()
}
function more() {
  clearTimeout(timer)
  timer = undefined
  void store.loadMemory(query.value, true)
}
function reloadSource() {
  if (store.memorySourceId) void select(store.memorySourceId, false, false)
}
function refresh() {
  search()
  reloadSource()
}
async function manage() {
  const id = store.memorySourceId,
    epoch = store.epoch
  await router.push({ path: '/memory/manage', query: { source_id: id } })
  if (epoch !== store.epoch || !store.connected) return
  store.clearInspection()
  store.advanced = 'memory'
  store.panel = 'advanced'
}
function shortcut(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    reading.value = false
    nextTick(() => input.value?.focus())
  }
}
function cancel(event: Event) {
  if (reading.value && narrow()) {
    event.preventDefault()
    event.stopImmediatePropagation()
    backToList()
  }
}
let dialog: HTMLDialogElement | null = null
onMounted(() => {
  document.addEventListener('keydown', shortcut)
  dialog = root.value?.closest('dialog') || null
  dialog?.addEventListener('cancel', cancel, true)
  if (!store.memorySourceId && store.memories[0])
    void select(store.memories[0].source_id)
})
onBeforeUnmount(() => {
  alive = false
  clearTimeout(timer)
  if (timer !== undefined && store.connected) void store.loadMemory(query.value)
  document.removeEventListener('keydown', shortcut)
  dialog?.removeEventListener('cancel', cancel, true)
})
</script>
