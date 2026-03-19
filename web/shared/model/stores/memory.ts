import { defineStore } from 'pinia'
import {
  fetchMemory,
  confirmMemory,
  rejectMemory,
  suppressMemory,
  unsuppressMemory,
  patchMemory,
  deleteMemory,
  type MemoryScopeView,
  type MemoryType,
  type ConfirmationState,
  type EffectiveStatus,
  type MemoryPatchRequest,
  type ProfileMemoryRecord,
  type EpisodicMemoryRecord,
  type ChunkMemoryRecord,
} from '~/entities/memory/api'

export interface MemoryFilters {
  scopeType: 'session' | 'user' | 'global'
  scopeID: string
  confirmationState: ConfirmationState | 'all'
  effectiveStatus: EffectiveStatus | 'all'
  showSuppressed: boolean
}

const defaultFilters = (): MemoryFilters => ({
  scopeType: 'session',
  scopeID: '',
  confirmationState: 'all',
  effectiveStatus: 'all',
  showSuppressed: false,
})

export const useMemoryStore = defineStore('memory', () => {
  const records = ref<MemoryScopeView | null>(null)
  const pending = ref(false)
  const actionPending = ref(false)
  const error = ref<string | null>(null)
  const filters = ref<MemoryFilters>(defaultFilters())

  const load = async (overrides: Partial<MemoryFilters> = {}) => {
    filters.value = {
      ...filters.value,
      ...overrides,
    }

    if (!filters.value.scopeID.trim()) {
      records.value = null
      error.value = null
      return
    }

    pending.value = true
    error.value = null

    try {
      records.value = await fetchMemory({
        scope_type: filters.value.scopeType,
        scope_id: filters.value.scopeID,
      })
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load memory'
    } finally {
      pending.value = false
    }
  }

  // Filtered lists based on current filter settings
  const filteredProfile = computed(() => {
    if (!records.value?.profile) return []
    return records.value.profile.filter(item => {
      if (filters.value.confirmationState !== 'all' && item.confirmation_state !== filters.value.confirmationState) {
        return false
      }
      if (filters.value.effectiveStatus !== 'all' && item.effective_status !== filters.value.effectiveStatus) {
        return false
      }
      if (!filters.value.showSuppressed && item.suppressed) {
        return false
      }
      return true
    })
  })

  const filteredEpisodic = computed(() => {
    if (!records.value?.episodic) return []
    return records.value.episodic.filter(item => {
      if (filters.value.confirmationState !== 'all' && item.confirmation_state !== filters.value.confirmationState) {
        return false
      }
      if (filters.value.effectiveStatus !== 'all' && item.effective_status !== filters.value.effectiveStatus) {
        return false
      }
      if (!filters.value.showSuppressed && item.suppressed) {
        return false
      }
      return true
    })
  })

  const filteredChunks = computed(() => {
    if (!records.value?.chunks) return []
    return records.value.chunks.filter(item => {
      // Chunks don't have confirmation_state, skip that filter
      if (filters.value.effectiveStatus !== 'all' && item.effective_status !== filters.value.effectiveStatus) {
        return false
      }
      if (!filters.value.showSuppressed && item.suppressed) {
        return false
      }
      return true
    })
  })

  // Helper to update a profile entry in local state
  const updateProfileEntry = (updated: ProfileMemoryRecord) => {
    if (!records.value?.profile) return
    const index = records.value.profile.findIndex(p => p.id === updated.id)
    if (index >= 0) {
      records.value.profile[index] = updated
    }
  }

  // Helper to update an episodic entry in local state
  const updateEpisodicEntry = (updated: EpisodicMemoryRecord) => {
    if (!records.value?.episodic) return
    const index = records.value.episodic.findIndex(e => e.id === updated.id)
    if (index >= 0) {
      records.value.episodic[index] = updated
    }
  }

  // Helper to update a chunk entry in local state
  const updateChunkEntry = (updated: ChunkMemoryRecord) => {
    if (!records.value?.chunks) return
    const index = records.value.chunks.findIndex(c => c.id === updated.id)
    if (index >= 0) {
      records.value.chunks[index] = updated
    }
  }

  // Helper to remove a chunk entry from local state (for hard delete)
  const removeChunkEntry = (id: number) => {
    if (!records.value?.chunks) return
    records.value.chunks = records.value.chunks.filter(c => c.id !== id)
  }

  // Confirm a memory entry
  const confirm = async (memoryType: MemoryType, id: number) => {
    actionPending.value = true
    error.value = null
    try {
      const result = await confirmMemory(memoryType, id)
      if (memoryType === 'profile') {
        updateProfileEntry(result as ProfileMemoryRecord)
      } else if (memoryType === 'episodic') {
        updateEpisodicEntry(result as EpisodicMemoryRecord)
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to confirm memory'
    } finally {
      actionPending.value = false
    }
  }

  // Reject a memory entry
  const reject = async (memoryType: MemoryType, id: number) => {
    actionPending.value = true
    error.value = null
    try {
      const result = await rejectMemory(memoryType, id)
      if (memoryType === 'profile') {
        updateProfileEntry(result as ProfileMemoryRecord)
      } else if (memoryType === 'episodic') {
        updateEpisodicEntry(result as EpisodicMemoryRecord)
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to reject memory'
    } finally {
      actionPending.value = false
    }
  }

  // Suppress a memory entry
  const suppress = async (memoryType: MemoryType, id: number) => {
    actionPending.value = true
    error.value = null
    try {
      const result = await suppressMemory(memoryType, id)
      if (memoryType === 'profile') {
        updateProfileEntry(result as ProfileMemoryRecord)
      } else if (memoryType === 'episodic') {
        updateEpisodicEntry(result as EpisodicMemoryRecord)
      } else if (memoryType === 'chunk') {
        updateChunkEntry(result as ChunkMemoryRecord)
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to suppress memory'
    } finally {
      actionPending.value = false
    }
  }

  // Unsuppress a memory entry
  const unsuppress = async (memoryType: MemoryType, id: number) => {
    actionPending.value = true
    error.value = null
    try {
      const result = await unsuppressMemory(memoryType, id)
      if (memoryType === 'profile') {
        updateProfileEntry(result as ProfileMemoryRecord)
      } else if (memoryType === 'episodic') {
        updateEpisodicEntry(result as EpisodicMemoryRecord)
      } else if (memoryType === 'chunk') {
        updateChunkEntry(result as ChunkMemoryRecord)
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to unsuppress memory'
    } finally {
      actionPending.value = false
    }
  }

  // Update (patch) a memory entry (only for editable types like profile)
  const patch = async (memoryType: MemoryType, id: number, data: MemoryPatchRequest) => {
    actionPending.value = true
    error.value = null
    try {
      const result = await patchMemory(memoryType, id, data)
      if (memoryType === 'profile') {
        updateProfileEntry(result)
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to update memory'
    } finally {
      actionPending.value = false
    }
  }

  // Delete a memory entry
  const remove = async (memoryType: MemoryType, id: number) => {
    actionPending.value = true
    error.value = null
    try {
      const result = await deleteMemory(memoryType, id)
      // For chunks (hard delete), remove from list. For others, update in place.
      if (memoryType === 'chunk') {
        removeChunkEntry(id)
      } else if (memoryType === 'profile') {
        updateProfileEntry(result as ProfileMemoryRecord)
      } else if (memoryType === 'episodic') {
        updateEpisodicEntry(result as EpisodicMemoryRecord)
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to delete memory'
    } finally {
      actionPending.value = false
    }
  }

  return {
    records,
    pending,
    actionPending,
    error,
    filters,
    filteredProfile,
    filteredEpisodic,
    filteredChunks,
    load,
    confirm,
    reject,
    suppress,
    unsuppress,
    patch,
    remove,
  }
})
