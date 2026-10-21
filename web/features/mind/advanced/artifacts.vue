<template><div class="mind-advanced">
  <main class="page">


    <div class="artifact-filters">
      <label>
        Type
        <input v-model="filters.type" type="text" placeholder="assistant_final" />
      </label>

      <label>
        Run ID
        <input v-model="filters.runID" type="text" placeholder="run-123" />
      </label>

      <label>
        Session key
        <input v-model="filters.sessionKey" type="text" placeholder="telegram:chat:123" />
      </label>

      <label>
        Search
        <input v-model="filters.query" type="text" placeholder="title, summary, content" />
      </label>

      <label>
        Limit
        <select v-model.number="filters.limit">
          <option :value="20">20</option>
          <option :value="50">50</option>
          <option :value="100">100</option>
        </select>
      </label>

      <div class="artifact-filters__actions">
        <AppButton variant="primary" :disabled="pending" @click="applyFilters">Apply</AppButton>
        <AppButton :disabled="pending" @click="resetFilters">Reset</AppButton>
      </div>
    </div>

    <p class="artifact-meta">Total: {{ total }}<span v-if="pending"> · loading...</span></p>

    <div class="artifact-pagination">
      <AppButton :disabled="pending || filters.offset <= 0" @click="prevPage">Prev</AppButton>
      <span>Offset: {{ filters.offset }}</span>
      <AppButton :disabled="pending || !canNext" @click="nextPage">Next</AppButton>
    </div>

    <AppAlert v-if="error" tone="error">Unable to load artifacts list.</AppAlert>
    <p v-else-if="pending" class="placeholder-text">Loading artifacts...</p>

    <AppEmptyState
      v-else-if="artifacts.length === 0"
      title="No artifacts found"
      description="Try changing type, run id, session key, or search query."
    />

    <AppTable v-else>
      <thead>
        <tr>
          <th>Title</th>
          <th>Type</th>
          <th>Run</th>
          <th>Session</th>
          <th>Created</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="artifact in artifacts" :key="artifact.artifact_id">
          <td>{{ artifact.title || '-' }}</td>
          <td>{{ artifact.artifact_type }}</td>
          <td>{{ artifact.run_id }}</td>
          <td>{{ artifact.session_key }}</td>
          <td>{{ formatDate(artifact.created_at) }}</td>
          <td>
            <AppButton :disabled="detailPending" @click="openPreview(artifact.artifact_id)">Preview</AppButton>
          </td>
        </tr>
      </tbody>
    </AppTable>

    <AppDialog :model-value="showPreview" @update:model-value="onDialogToggle">
      <template #title>
        <div class="preview-title">Artifact preview</div>
      </template>

      <AppAlert v-if="detailError" tone="error">Unable to load artifact details.</AppAlert>
      <p v-else-if="detailPending" class="placeholder-text">Loading artifact details...</p>

      <template v-else-if="selected">
        <div class="preview-meta">
          <p><strong>ID:</strong> {{ selected.artifact_id }}</p>
          <p><strong>Type:</strong> {{ selected.artifact_type }}</p>
          <p><strong>Run:</strong> {{ selected.run_id }}</p>
          <p><strong>Session:</strong> {{ selected.session_key }}</p>
        </div>

        <AppPanel title="Summary">
          <p class="preview-text">{{ selected.summary || '-' }}</p>
        </AppPanel>

        <AppPanel title="Content" class="preview-panel">
          <pre class="preview-content">{{ previewContent }}</pre>
        </AppPanel>
      </template>
    </AppDialog>
  </main>
</div></template>

<script setup lang="ts">
const mindPanelProps=defineProps<{ownerToken?:string}>()
import { storeToRefs } from 'pinia'
import AppAlert from '~/shared/ui/AppAlert.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppDialog from '~/shared/ui/AppDialog.vue'
import AppEmptyState from '~/shared/ui/AppEmptyState.vue'
import AppPanel from '~/shared/ui/AppPanel.vue'
import AppTable from '~/shared/ui/AppTable.vue'
import { useArtifactsStore } from '~/shared/model/stores/artifacts'

const artifactsStore = useArtifactsStore()
const route = useRoute()
const {
  items: artifacts,
  total,
  pending,
  error,
  filters,
  selected,
  detailPending,
  detailError,
} = storeToRefs(artifactsStore)

const canNext = computed(() => {
  return artifacts.value.length >= filters.value.limit
})

const showPreview = computed(() => {
  return Boolean(artifactsStore.selectedID)
})

const previewContent = computed(() => {
  if (!selected.value) {
    return ''
  }

  if (selected.value.content_text.trim()) {
    return selected.value.content_text
  }

  return selected.value.content_json || '{}'
})

const formatDate = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return date.toLocaleString()
}

const applyFilters = async () => {
  await artifactsStore.load({
    type: filters.value.type,
    runID: filters.value.runID,
    sessionKey: filters.value.sessionKey,
    query: filters.value.query,
    limit: filters.value.limit,
    offset: 0,
  })
}

const resetFilters = async () => {
  filters.value.type = ''
  filters.value.runID = ''
  filters.value.sessionKey = ''
  filters.value.query = ''
  filters.value.limit = 50
  await artifactsStore.load({ offset: 0 })
}

const prevPage = async () => {
  const nextOffset = Math.max(0, filters.value.offset - filters.value.limit)
  await artifactsStore.load({ offset: nextOffset })
}

const nextPage = async () => {
  await artifactsStore.load({ offset: filters.value.offset + filters.value.limit })
}

const openPreview = async (artifactID: string) => {
  await artifactsStore.openPreview(artifactID)
}

const onDialogToggle = (value: boolean) => {
  if (!value) {
    artifactsStore.closePreview()
  }
}

onMounted(async () => {
  await artifactsStore.load({ offset: 0 })
})
watch(() => route.query.artifact_id, async value => {
  if (typeof value === 'string' && /^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,255}$/.test(value)) await openPreview(value)
  else artifactsStore.closePreview()
}, { immediate: true })
onBeforeUnmount(() => artifactsStore.closePreview())
</script>
