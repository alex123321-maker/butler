<template>
  <template v-for="entry in entries" :key="entry.id">
    <div v-if="entry.input" class="mind-user-message"><span class="sr-only">Вы</span><p>{{ entry.input.content }}</p></div>
    <MindPublication v-else-if="entry.publication" :publication="entry.publication" />
    <div v-else-if="entry.notice" class="mind-execution-notice" :data-execution-notice="entry.notice.input_ref"
      :role="['blocked', 'failed', 'missing_response'].includes(entry.notice.code) ? 'alert' : 'status'" aria-live="polite">
      <p>{{ entry.notice.text }}</p>
      <button v-if="entry.notice.approval_ref" type="button" class="mind-text-button" @click="store.panel = 'approvals'">Открыть подтверждения</button>
    </div>
  </template>
  <p v-if="store.communicationError" class="mind-error" role="alert">{{ store.communicationError }}</p>
  <p v-if="store.executionNoticeError" class="mind-error" role="status">{{ store.executionNoticeError }}</p>
  <button v-if="store.publicationMore" class="mind-text-button" type="button" @click="store.loadPublications(true)">Загрузить продолжение разговора</button>
  <MindCommunicationDiagnostics />
</template>
<script setup lang="ts">
import { useMind } from '../model/store'
import type { Publication, PublicationInput, ExecutionNotice } from '../model/communications'
import MindPublication from './MindPublication.vue'
import MindCommunicationDiagnostics from './MindCommunicationDiagnostics.vue'
const store = useMind()
const entries = computed(() => {
  const rows: { id: string; at: string; input?: PublicationInput; publication?: Publication; notice?: ExecutionNotice }[] = [
    ...store.publicationInputs.map(input => ({ id: input.message_id, at: input.created_at, input })),
    ...store.publications.map(publication => ({ id: publication.publication_ref, at: publication.created_at, publication })),
    ...store.executionNotices.filter(notice => notice.code !== 'finished').map(notice => ({ id: notice.notice_ref, at: notice.created_at, notice })),
  ]
  return rows.sort((a, b) => Date.parse(a.at) - Date.parse(b.at))
})
</script>
<style scoped>
.mind-execution-notice {
  padding: 12px 16px;
  border-left: 3px solid currentColor;
  opacity: .85;
}

.mind-execution-notice p {
  margin: 0;
}

.mind-execution-notice[role="alert"] {
  color: var(--color-state-error);
}
</style>
