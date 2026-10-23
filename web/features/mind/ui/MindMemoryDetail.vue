<template>
  <article v-if="source" class="memory-detail">
    <div class="memory-detail-top">
      <span class="mind-source-caption">{{ memoryOrigin(source) }}</span
      ><button class="mind-text-button" type="button" @click="emit('history')">
        История
      </button>
    </div>
    <h3>{{ source.title || 'Без названия' }}</h3>
    <span class="memory-status">{{ sourceStatus(source) }}</span>
    <template v-if="readableSource(source)">
      <p class="memory-body">{{ source.body || 'В источнике нет текста.' }}</p>
      <section class="memory-detail-section">
        <h4>Источник</h4>
        <p class="mind-muted">
          {{ memoryOrigin(source) }} ·
          {{ dateLabel(source.event_time || source.extracted_at) }} · версия
          {{ source.revision }}
        </p>
        <button class="mind-text-button" type="button" @click="emit('source')">
          Открыть источник ↗
        </button>
      </section>
      <section class="memory-detail-section">
        <div class="memory-detail-top">
          <h4>Связанные записи · {{ related.length }}</h4>
          <button
            v-if="related.length"
            class="mind-text-button"
            type="button"
            @click="emit('relations')"
          >
            Почему связаны?
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
        <p v-if="!related.length" class="mind-muted">
          В загруженных данных нет ссылок на другие записи.
        </p>
      </section>
      <section class="memory-detail-section">
        <h4>Факты из источника</h4>
        <p v-if="claimsBusy" class="mind-muted">Проверяю подтверждения…</p>
        <p v-else-if="claimsError" class="mind-muted">{{ claimsError }}</p>
        <p v-else-if="!claims.length" class="mind-muted">
          В загруженной части каталога нет фактов для этой версии. Сохранение
          источника не подтверждает его содержание.
        </p>
        <div v-for="claim in claims" :key="claim.claim_id" class="memory-claim">
          <p>{{ claim.summary }}</p>
          <small>{{ claimStatus(claim.status) }}</small>
          <button
            v-if="claim.status === 'candidate'"
            class="mind-button"
            type="button"
            :disabled="deciding"
            @click="emit('confirm', claim)"
          >
            Подтвердить факт
          </button>
        </div>
      </section>
      <p class="mind-muted">
        Использование в ответах зависит от доступа и текущих правил памяти.
      </p>
      <button class="mind-text-button" type="button" @click="emit('manage')">
        Хранилище и политики памяти ↗
      </button>
    </template>
    <p v-else class="mind-muted">
      Содержимое этого источника недоступно для просмотра.
    </p>
  </article>
</template>
<script setup lang="ts">
import type { MemoryClaim, Source } from '../model/types'
import { readableSource, sourceStatus } from '../model/memory-explorer'
import { dateLabel, memoryOrigin } from '../model/presentation'
defineProps<{
  source: Source
  related: { id: string; title: string; reason: string }[]
  claims: MemoryClaim[]
  claimsBusy: boolean
  claimsError: string
  deciding: boolean
}>()
const emit = defineEmits<{
  history: []
  source: []
  relations: []
  select: [id: string]
  confirm: [claim: MemoryClaim]
  manage: []
}>()
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
