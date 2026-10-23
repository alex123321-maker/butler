<template>
  <article class="memory-detail">
    <p class="mind-source-caption">
      {{ memoryOrigin(source) }} ·
      {{ dateLabel(source.event_time || source.extracted_at) }}
    </p>
    <h3 tabindex="-1">{{ source.title || 'Без названия' }}</h3>
    <template v-if="readableSource(source)">
      <p class="memory-body">{{ source.body || 'В источнике нет текста.' }}</p>
      <div class="memory-reading-actions">
        <button class="mind-text-button" type="button" @click="copy">
          Копировать текст
        </button>
        <span v-if="copyStatus" role="status">{{ copyStatus }}</span>
      </div>
      <section
        v-if="claims.length || claimsError"
        class="memory-detail-section"
        aria-label="Факты из источника"
      >
        <h4>Факты из этой записи</h4>
        <p v-if="claimsError" class="mind-error" role="alert">
          {{ claimsError }}
          <button
            class="mind-text-button"
            type="button"
            @click="emit('refresh')"
          >
            Проверить состояние
          </button>
        </p>
        <div v-for="claim in claims" :key="claim.claim_id" class="memory-claim">
          <p>{{ claim.summary }}</p>
          <small>{{ claimStatus(claim.status) }}</small>
          <div
            v-if="claim.status === 'candidate'"
            class="memory-reading-actions"
          >
            <button
              class="mind-button"
              type="button"
              :disabled="deciding"
              @click="emit('decide', claim, false)"
            >
              Подтвердить факт
            </button>
            <button
              class="mind-text-button"
              type="button"
              :disabled="deciding"
              @click="emit('decide', claim, true)"
            >
              Отклонить факт
            </button>
          </div>
        </div>
      </section>
      <section v-if="related.length" class="memory-detail-section">
        <div class="memory-detail-top">
          <h4>Связанные записи · {{ related.length }}</h4>
          <button class="mind-text-button" type="button" @click="emit('graph')">
            Показать на схеме
          </button>
        </div>
        <button
          v-for="item in related"
          :key="item.id"
          class="memory-related"
          type="button"
          @click="emit('select', item.id)"
        >
          <span>{{ item.title }}</span
          ><small>{{ item.reason }}</small>
        </button>
      </section>
      <details class="memory-provenance">
        <summary>Источник и история</summary>
        <p class="mind-muted">
          {{ sourceStatus(source) }} · версия {{ source.revision }}. Сохранение
          записи не означает подтверждение её содержания.
        </p>
        <p v-if="claimsBusy" class="mind-muted">Проверяю факты…</p>
        <p v-else-if="!claims.length && !claimsError" class="mind-muted">
          Для этой версии в доступной части каталога нет отдельных фактов.
        </p>
        <p class="mind-muted">
          Добавлено {{ dateLabel(source.extracted_at) }}. Связи показывают
          сохранённое происхождение; ещё не загруженные записи могут дополнять
          этот список.
        </p>
        <p class="memory-source-id">{{ source.source_id }}</p>
        <details v-if="inspection.fragments?.length">
          <summary>
            Фрагменты источника · {{ inspection.fragments.length }}
          </summary>
          <div
            v-for="fragment in inspection.fragments"
            :key="fragment.chunk_id"
          >
            <h4>{{ fragment.title }}</h4>
            <blockquote class="memory-body">{{ fragment.text }}</blockquote>
          </div>
        </details>
        <details>
          <summary>
            История версий · {{ inspection.revisions?.length || 0 }}
          </summary>
          <details
            v-for="revision in inspection.revisions || []"
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
          <p v-if="!inspection.revisions?.length" class="mind-muted">
            История версий пока недоступна.
          </p>
          <p
            v-if="
              inspection.revision_limit &&
              (inspection.revisions?.length || 0) >= inspection.revision_limit
            "
            class="mind-muted"
          >
            Показаны последние {{ inspection.revision_limit }} версий.
          </p>
        </details>
        <button
          class="mind-text-button memory-manage"
          type="button"
          @click="emit('manage')"
        >
          Управлять этой записью ↗
        </button>
      </details>
    </template>
    <p v-else class="mind-muted">
      {{ sourceStatus(source) }}. Содержимое этого источника недоступно для
      просмотра.
    </p>
  </article>
</template>
<script setup lang="ts">
import type { Inspection, MemoryClaim } from '../model/types'
import { readableSource, sourceStatus } from '../model/memory-explorer'
import { dateLabel, memoryOrigin } from '../model/presentation'
const props = defineProps<{
  inspection: Inspection
  related: { id: string; title: string; reason: string }[]
  claims: MemoryClaim[]
  claimsBusy: boolean
  claimsError: string
  deciding: boolean
}>()
const emit = defineEmits<{
  select: [id: string]
  decide: [claim: MemoryClaim, reject: boolean]
  manage: []
  graph: []
  refresh: []
}>()
const source = computed(() => props.inspection.source)
const copyStatus = ref('')
let alive = true
async function copy() {
  copyStatus.value = ''
  try {
    await navigator.clipboard.writeText(source.value.body)
    if (alive) copyStatus.value = 'Текст скопирован'
  } catch {
    if (alive)
      copyStatus.value = 'Не удалось скопировать. Выделите нужный текст.'
  }
}
onBeforeUnmount(() => {
  alive = false
})
const claimStatus = (status: string) =>
  (
    ({
      candidate: 'Нужно ваше подтверждение',
      confirmed: 'Подтверждено вами',
      rejected: 'Отклонено',
      superseded: 'Заменено новым фактом',
    }) as Record<string, string>
  )[status] || status
</script>
