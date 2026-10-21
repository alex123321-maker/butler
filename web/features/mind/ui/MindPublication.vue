<template>
  <article class="mind-publication" :data-publication="publication.publication_ref">
    <p v-if="!publication.available || !publication.payload" class="mind-muted">Публикация недоступна: доступ или содержимое изменились.</p>
    <template v-else>
      <h2 v-if="publication.payload.title">{{ publication.payload.title }}</h2>
      <p v-if="publication.payload.summary" class="mind-readable">{{ publication.payload.summary }}</p>
      <p v-if="publication.payload.text" class="mind-readable">{{ publication.payload.text }}</p>
      <template v-for="image in publication.payload.images || []" :key="image.artifact_ref">
        <MindPublicationMedia v-if="file(image.artifact_ref)" :publication="publication.publication_ref" :file="file(image.artifact_ref)!" :alt="image.alt" :caption="image.caption" image />
        <p v-else class="mind-muted">Изображение недоступно.</p>
      </template>
      <template v-for="(block, index) in publication.payload.blocks || []" :key="index">
        <h3 v-if="block.type === 'heading'">{{ block.text }}</h3>
        <p v-else-if="block.type === 'paragraph'" class="mind-readable">{{ block.text }}</p>
        <blockquote v-else-if="block.type === 'quote'">{{ block.text }}</blockquote>
        <pre v-else-if="block.type === 'code'"><code>{{ block.text }}</code></pre>
        <ul v-else-if="block.type === 'list'"><li v-for="(item, i) in block.items || []" :key="i">{{ item }}</li></ul>
        <div v-else-if="block.type === 'table'" class="mind-publication-table" role="region" aria-label="Таблица отчёта" tabindex="0">
          <table><tbody><tr v-for="(row, r) in block.rows || []" :key="r"><component :is="r === 0 ? 'th' : 'td'" v-for="(cell, c) in row" :key="c" :scope="r === 0 ? 'col' : undefined">{{ cell }}</component></tr></tbody></table>
        </div>
        <hr v-else-if="block.type === 'divider'" />
        <template v-else-if="block.type === 'image' || block.type === 'document'">
          <MindPublicationMedia v-if="file(block.artifact_ref)" :publication="publication.publication_ref" :file="file(block.artifact_ref)!" :alt="block.alt" :image="block.type === 'image'" />
          <p v-else class="mind-muted">Вложение недоступно.</p>
        </template>
        <p v-else class="mind-error">Этот блок не поддерживается.</p>
      </template>
      <template v-for="ref in publication.payload.attachments || []" :key="ref">
        <MindPublicationMedia v-if="file(ref)" :publication="publication.publication_ref" :file="file(ref)!" />
        <p v-else class="mind-muted">Вложение недоступно.</p>
      </template>
      <small class="mind-muted">Доступно в Butler · {{ dateLabel(publication.created_at) }}</small>
      <button v-if="publication.task_ref" class="mind-text-button" type="button" @click="store.selectTask(publication.task_ref!, true)">{{ store.tasks.find(task => task.task_id === publication.task_ref)?.goal || 'Открыть исходное поручение' }}</button>
      <button class="mind-text-button" type="button" :disabled="store.communicationDraining" @click="reply">Ответить на публикацию</button>
      <details class="mind-publication-delivery"><summary>Доставка и части</summary>
        <p v-for="part in publication.parts" :key="`${part.binding_id}-${part.binding_version}-${part.index}`">
          {{ part.channel === 'web' ? 'Веб' : 'Telegram' }} · часть {{ part.index + 1 }} · {{ states[part.state] || 'Состояние неизвестно' }}
        </p>
        <details><summary>Сохранённый состав</summary><pre>{{ JSON.stringify(publication.parts, null, 2) }}</pre></details>
      </details>
    </template>
  </article>
</template>
<script setup lang="ts">
import type { Publication } from '../model/communications'
import { dateLabel } from '../model/presentation'
import { useMind } from '../model/store'
import MindPublicationMedia from './MindPublicationMedia.vue'
const props = defineProps<{ publication: Publication }>()
const store = useMind()
const file = (ref?: string) => props.publication.files.find(f => f.artifact_ref === ref)
const states: Record<string, string> = { available: 'доступно', pending: 'ожидает отправки', sending: 'отправляется', sent: 'канал принял', delivery_unknown: 'результат неизвестен', failed: 'доставка не подтверждена' }
function reply() { store.replyPublication = props.publication; nextTick(() => document.getElementById('mind-message')?.focus()) }
</script>
<style scoped>
.mind-publication { min-width: 0; overflow-wrap: anywhere; padding-block: 12px; }
.mind-publication h2 { font-size: 20px; }
.mind-publication pre { max-width: 100%; overflow: auto; white-space: pre-wrap; }
.mind-publication-table { max-width: 100%; overflow-x: auto; }
.mind-publication table { width: 100%; border-collapse: collapse; }
.mind-publication th, .mind-publication td { padding: 8px; border: 1px solid var(--color-border-subtle); text-align: left; white-space: pre-wrap; }
.mind-publication small { display: block; }
.mind-publication-delivery { margin-top: 8px; }
</style>
