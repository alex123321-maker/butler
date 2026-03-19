import { defineStore } from 'pinia'
import { fetchArtifactById, fetchArtifacts, type ArtifactItem } from '~/entities/artifact/api'

export interface ArtifactFilters {
  type: string
  runID: string
  sessionKey: string
  query: string
  limit: number
  offset: number
}

const defaultFilters = (): ArtifactFilters => ({
  type: '',
  runID: '',
  sessionKey: '',
  query: '',
  limit: 50,
  offset: 0,
})

export const useArtifactsStore = defineStore('artifacts', () => {
  const items = ref<ArtifactItem[]>([])
  const total = ref(0)
  const pending = ref(false)
  const error = ref<string | null>(null)
  const filters = ref<ArtifactFilters>(defaultFilters())

  const selected = ref<ArtifactItem | null>(null)
  const selectedID = ref('')
  const detailPending = ref(false)
  const detailError = ref<string | null>(null)

  const load = async (overrides: Partial<ArtifactFilters> = {}) => {
    filters.value = {
      ...filters.value,
      ...overrides,
    }

    pending.value = true
    error.value = null

    try {
      const response = await fetchArtifacts({
        type: filters.value.type || undefined,
        run_id: filters.value.runID || undefined,
        session_key: filters.value.sessionKey || undefined,
        query: filters.value.query || undefined,
        limit: filters.value.limit,
        offset: filters.value.offset,
      })

      items.value = response.artifacts ?? []
      total.value = items.value.length
    } catch (err) {
      items.value = []
      total.value = 0
      error.value = err instanceof Error ? err.message : 'Failed to load artifacts'
    } finally {
      pending.value = false
    }
  }

  const openPreview = async (artifactID: string) => {
    selectedID.value = artifactID
    selected.value = null
    detailPending.value = true
    detailError.value = null

    try {
      const response = await fetchArtifactById(artifactID)
      selected.value = response.artifact
    } catch (err) {
      selected.value = null
      detailError.value = err instanceof Error ? err.message : 'Failed to load artifact details'
    } finally {
      detailPending.value = false
    }
  }

  const closePreview = () => {
    selectedID.value = ''
    selected.value = null
    detailError.value = null
  }

  return {
    items,
    total,
    pending,
    error,
    filters,
    selected,
    selectedID,
    detailPending,
    detailError,
    load,
    openPreview,
    closePreview,
  }
})
