<template>
  <section class="memory-explorer" aria-label="Просмотр памяти">
    <form class="memory-search" role="search" @submit.prevent="search">
      <MindIcon name="search" />
      <input
        ref="input"
        v-model="query"
        type="search"
        maxlength="500"
        aria-label="Поиск записей"
        placeholder="Что вспомнить?"
        @input="queueSearch"
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
    <div class="memory-explorer-bar">
      <div class="memory-views" role="group" aria-label="Представление памяти">
        <button
          class="mind-text-button"
          type="button"
          :aria-pressed="view === 'list'"
          @click="view = 'list'"
        >
          Записи
        </button>
        <button
          class="mind-text-button"
          type="button"
          :aria-pressed="view === 'graph'"
          @click="view = 'graph'"
        >
          Граф
        </button>
      </div>
      <span role="status">{{
        store.memoryBusy
          ? 'Ищу…'
          : store.memoryQuery.trim()
            ? `Найдено: ${store.memories.length}${store.memoryHasMore ? '+' : ''} · По тексту`
            : `Загружено: ${store.memories.length}`
      }}</span>
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
      <button class="mind-text-button" type="button" @click="search">
        Повторить
      </button>
    </p>
    <div class="memory-workspace">
      <div class="memory-results" :aria-busy="store.memoryBusy">
        <div
          v-if="view === 'list'"
          class="memory-list"
          aria-label="Записи памяти"
        >
          <button
            v-for="source in orderedSources"
            :key="source.source_id"
            class="memory-list-row"
            :class="{ selected: source.source_id === store.memorySourceId }"
            type="button"
            :aria-pressed="source.source_id === store.memorySourceId"
            @click="select(source.source_id)"
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
                  pins.includes(source.source_id)
                    ? ' · Закреплено в просмотре'
                    : ''
                }}</small
              ></span
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
          @select="select"
        />
        <p
          v-if="
            !store.memoryBusy && !store.memoryError && !store.memories.length
          "
          class="memory-empty"
        >
          {{
            query.trim()
              ? 'Ничего не найдено. Попробуйте другие слова.'
              : 'Память пока пуста.'
          }}
        </p>
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
          Достигнут предел просмотра. Уточните поисковый запрос.
        </p>
      </div>
      <aside
        class="memory-inspector"
        aria-label="Выбранная запись"
        :aria-busy="store.memoryDetailBusy"
      >
        <p v-if="store.memoryDetailBusy" class="memory-empty" role="status">
          Открываю запись…
        </p>
        <p
          v-else-if="store.memoryDetailError"
          class="memory-empty"
          role="alert"
        >
          {{ store.memoryDetailError }}
          <button
            class="mind-text-button"
            type="button"
            @click="select(store.memorySourceId)"
          >
            Повторить
          </button>
        </p>
        <template v-else-if="store.inspection">
          <div class="memory-selection-tools">
            <button
              class="mind-text-button"
              type="button"
              :aria-pressed="pins.includes(store.memorySourceId)"
              title="Закрепление действует до закрытия просмотра и не меняет память агента"
              @click="pin"
            >
              {{
                pins.includes(store.memorySourceId)
                  ? 'Открепить'
                  : 'Закрепить в просмотре'
              }}
            </button>
          </div>
          <div class="memory-desktop-detail">
            <MindMemoryDetail v-bind="detailProps" v-on="detailEvents" />
          </div>
          <div class="memory-mobile-summary">
            <strong>{{
              store.inspection.source.title || 'Без названия'
            }}</strong
            ><small>{{ sourceStatus(store.inspection.source) }}</small
            ><button
              class="mind-button"
              type="button"
              @click="overlay = 'detail'"
            >
              Открыть запись
            </button>
          </div>
        </template>
        <p v-else class="memory-empty">
          Выберите запись, чтобы прочитать её и узнать, откуда она появилась.
        </p>
      </aside>
    </div>
    <footer class="memory-explorer-footer">
      <span
        >Данные памяти Butler · {{ store.memories.length }} записей{{
          store.memoryHasMore ? ' · есть ещё' : ''
        }}</span
      ><span>Поиск по тексту · смысловой поиск через Батлера</span>
    </footer>
    <MindDialog :open="!!overlay" :title="overlayTitle" @close="overlay = ''">
      <template v-if="overlay === 'detail' && store.inspection"
        ><button
          class="mind-text-button"
          type="button"
          :aria-pressed="pins.includes(store.memorySourceId)"
          @click="pin"
        >
          {{
            pins.includes(store.memorySourceId)
              ? 'Открепить'
              : 'Закрепить в просмотре'
          }}</button
        ><MindMemoryDetail v-bind="detailProps" v-on="detailEvents"
      /></template>
      <template v-else-if="overlay === 'history' && store.inspection">
        <p class="mind-muted">
          Сохранённые версии источника. История не является подтверждением
          фактов.
        </p>
        <template v-if="readableSource(store.inspection.source)">
          <details
            v-for="revision in store.inspection.revisions || []"
            :key="revision.revision"
            class="memory-revision"
          >
            <summary>
              Версия {{ revision.revision }} ·
              {{ dateLabel(revision.event_time || revision.created_at) }}
            </summary>
            <p class="memory-body">
              {{
                revision.origin?.sensitivity === 'secret'
                  ? 'Закрытое содержимое'
                  : revision.body
              }}
            </p>
          </details>
          <p v-if="!store.inspection.revisions?.length" class="mind-muted">
            История версий пока недоступна.
          </p>
          <p
            v-if="
              store.inspection.revision_limit &&
              (store.inspection.revisions?.length || 0) >=
                store.inspection.revision_limit
            "
            class="mind-muted"
          >
            Показаны последние {{ store.inspection.revision_limit }} версий.
          </p>
        </template>
        <p v-else class="mind-muted">История содержимого недоступна.</p>
      </template>
      <template
        v-else-if="
          overlay === 'source' &&
          store.inspection &&
          readableSource(store.inspection.source)
        "
      >
        <p class="mind-source-caption">
          {{ memoryOrigin(store.inspection.source) }} · версия
          {{ store.inspection.source.revision }}
        </p>
        <p class="memory-body">{{ store.inspection.source.body }}</p>
        <details v-if="store.inspection.fragments?.length">
          <summary>Фрагменты · {{ store.inspection.fragments.length }}</summary>
          <div
            v-for="fragment in store.inspection.fragments"
            :key="fragment.chunk_id"
          >
            <h3>{{ fragment.title }}</h3>
            <blockquote class="memory-body">{{ fragment.text }}</blockquote>
          </div>
        </details>
        <p class="memory-source-id">{{ store.inspection.source.source_id }}</p>
        <p class="mind-muted">
          {{
            sourceParents(store.inspection.source).length
              ? 'Исходные записи доступны в связях.'
              : 'Это сохранённый источник. Отдельная исходная цитата в данных не указана.'
          }}
        </p>
      </template>
      <template v-else-if="overlay === 'relations'">
        <p class="mind-muted">
          Связи взяты из происхождения записей. Они не означают, что содержимое
          подтверждено или что источники совпадают по смыслу.
        </p>
        <button
          v-for="item in related"
          :key="item.id"
          class="memory-related"
          type="button"
          @click="select(item.id)"
        >
          <strong>{{ item.title }}</strong
          ><small>{{ item.reason }}</small>
        </button>
      </template>
    </MindDialog>
  </section>
</template>
<script setup lang="ts">
import { useMind } from '../model/store'
import type { MemoryClaim } from '../model/types'
import {
  readableSource,
  sourceParents,
  sourceStatus,
} from '../model/memory-explorer'
import { dateLabel, memoryOrigin, memoryIcon } from '../model/presentation'
import MindIcon from './MindIcon.vue'
import MindDialog from './MindDialog.vue'
import MindMemoryDetail from './MindMemoryDetail.vue'
import MindMemoryGraph from './MindMemoryGraph.vue'
const store = useMind()
const query = ref(store.memoryQuery),
  view = ref<'list' | 'graph'>('list'),
  pins = ref<string[]>([])
const overlay = ref(''),
  input = ref<HTMLInputElement | null>(null)
const claims = ref<MemoryClaim[]>([]),
  claimsBusy = ref(false),
  claimsError = ref(''),
  deciding = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined,
  claimVersion = 0,
  alive = true
const orderedSources = computed(() =>
  [...store.memories].sort(
    (a, b) =>
      Number(pins.value.includes(b.source_id)) -
      Number(pins.value.includes(a.source_id)),
  ),
)
const related = computed(() => {
  const source = store.inspection?.source
  if (!source || !readableSource(source)) return []
  const result = sourceParents(source).map((id) => ({
    id,
    title:
      store.memories.find((s) => s.source_id === id)?.title ||
      'Источник вне списка',
    reason: 'Сохранённая ссылка происхождения',
  }))
  for (const s of store.memories) {
    if (
      sourceParents(s).includes(source.source_id) &&
      !result.some((r) => r.id === s.source_id)
    )
      result.push({
        id: s.source_id,
        title: s.title || 'Без названия',
        reason: 'Эта запись ссылается на выбранный источник',
      })
  }
  return result
})
const detailProps = computed(() => ({
  source: store.inspection!.source,
  related: related.value,
  claims: claims.value.filter(
    (c) =>
      c.source_id === store.memorySourceId &&
      c.source_revision === store.inspection?.source.revision,
  ),
  claimsBusy: claimsBusy.value,
  claimsError: claimsError.value,
  deciding: deciding.value || claimsBusy.value || !!claimsError.value,
}))
const detailEvents = {
  history: () => {
    overlay.value = 'history'
  },
  source: () => {
    overlay.value = 'source'
  },
  relations: () => {
    overlay.value = 'relations'
  },
  select,
  confirm,
  manage,
}
const overlayTitle = computed(
  () =>
    (
      ({
        history: 'История записи',
        source: 'Сохранённый источник',
        relations: 'Почему связаны?',
        detail: 'Запись памяти',
      }) as Record<string, string>
    )[overlay.value] || 'Память',
)
async function select(id: string) {
  overlay.value = ''
  await store.inspectMemory(id, false)
}
function pin() {
  const id = store.memorySourceId
  pins.value = pins.value.includes(id)
    ? pins.value.filter((p) => p !== id)
    : [...pins.value, id]
}
function queueSearch() {
  clearTimeout(timer)
  // Fence in-flight responses immediately, before the debounce expires.
  store.memoryQuery = query.value
  store.memories = []
  store.memoryHasMore = false
  store.memoryBusy = true
  timer = setTimeout(search, 240)
}
function search() {
  clearTimeout(timer)
  timer = undefined
  void store.loadMemory(query.value)
}
function clear() {
  query.value = ''
  search()
  input.value?.focus()
}
function more() {
  clearTimeout(timer)
  timer = undefined
  void store.loadMemory(query.value, true)
}
function refresh() {
  search()
  if (store.memorySourceId) void select(store.memorySourceId)
}
function manage() {
  overlay.value = ''
  store.clearInspection()
  store.advanced = 'memory'
  store.panel = 'advanced'
}
async function loadClaims() {
  const version = ++claimVersion,
    epoch = store.epoch
  claims.value = []
  claimsError.value = ''
  claimsBusy.value = true
  try {
    const result = await store.request<{
      claims: MemoryClaim[]
      claim_list_limit: number
    }>('/api/v4/memory')
    if (!alive || epoch !== store.epoch || version !== claimVersion) return
    claims.value = result.claims || []
    if (claims.value.length >= result.claim_list_limit)
      claimsError.value =
        'Показана ограниченная часть каталога фактов. Полная история доступна в хранилище.'
  } catch {
    if (alive && epoch === store.epoch && version === claimVersion)
      claimsError.value =
        'Подтверждения сейчас недоступны. Обновите запись, чтобы повторить.'
  } finally {
    if (alive && epoch === store.epoch && version === claimVersion)
      claimsBusy.value = false
  }
}
async function confirm(claim: MemoryClaim) {
  if (
    deciding.value ||
    claimsBusy.value ||
    claimsError.value ||
    claim.source_revision !== store.inspection?.source.revision
  )
    return
  const epoch = store.epoch,
    id = store.memorySourceId,
    version = claimVersion
  const active = claims.value.find(
    (c) =>
      c.status === 'confirmed' &&
      c.key === claim.key &&
      c.scope_id === claim.scope_id &&
      c.scope_type === claim.scope_type,
  )
  deciding.value = true
  try {
    await store.request('/api/v4/memory/decisions', {
      claim_id: claim.claim_id,
      expected_active_id: active?.claim_id || '',
      reject: false,
    })
    if (
      alive &&
      epoch === store.epoch &&
      version === claimVersion &&
      id === store.memorySourceId
    )
      await loadClaims()
  } catch {
    if (alive && epoch === store.epoch && version === claimVersion)
      claimsError.value =
        'Решение не подтверждено. Обновите запись перед повтором.'
  } finally {
    if (alive && epoch === store.epoch) deciding.value = false
  }
}
function shortcut(event: KeyboardEvent) {
  if (
    (event.ctrlKey || event.metaKey) &&
    event.key.toLowerCase() === 'k' &&
    !overlay.value
  ) {
    event.preventDefault()
    input.value?.focus()
  }
}
watch(
  () => store.memorySourceId,
  () => {
    claims.value = []
    claimsError.value = ''
    claimVersion++
  },
)
watch(
  () => store.inspection,
  (inspection) => {
    if (inspection && readableSource(inspection.source)) void loadClaims()
  },
  { immediate: true },
)
onMounted(() => {
  document.addEventListener('keydown', shortcut)
  if (!store.memorySourceId && store.memories[0])
    void select(store.memories[0].source_id)
})
onBeforeUnmount(() => {
  alive = false
  claimVersion++
  clearTimeout(timer)
  if (timer !== undefined && store.connected) void store.loadMemory(query.value)
  document.removeEventListener('keydown', shortcut)
})
</script>
