<template>
  <template v-for="entry in entries" :key="entry.id">
    <div v-if="entry.input" class="mind-user-message"><span class="sr-only">Вы</span><p>{{ entry.input.content }}</p></div>
    <MindPublication v-else-if="entry.publication" :publication="entry.publication" />
  </template>
  <p v-if="store.communicationError" class="mind-error" role="alert">{{ store.communicationError }}</p>
  <button v-if="store.publicationMore" class="mind-text-button" type="button" @click="store.loadPublications(true)">Загрузить продолжение разговора</button>
</template>
<script setup lang="ts">
import { useMind } from '../model/store'
import type { Publication, PublicationInput } from '../model/communications'
import MindPublication from './MindPublication.vue'
const store = useMind()
const entries = computed(() => {
  const rows: { id: string; at: string; input?: PublicationInput; publication?: Publication }[] = [
    ...store.publicationInputs.map(input => ({ id: input.message_id, at: input.created_at, input })),
    ...store.publications.map(publication => ({ id: publication.publication_ref, at: publication.created_at, publication })),
  ]
  return rows.sort((a, b) => Date.parse(a.at) - Date.parse(b.at))
})
</script>
