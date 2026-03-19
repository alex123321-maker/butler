import { defineStore } from 'pinia'
import { fetchSystemSummary, type SystemSummaryResponse } from '~/entities/system/api'

export const useSystemStore = defineStore('system', () => {
  const summary = ref<SystemSummaryResponse | null>(null)
  const pending = ref(false)
  const error = ref<string | null>(null)

  const load = async () => {
    pending.value = true
    error.value = null

    try {
      summary.value = await fetchSystemSummary()
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load system summary'
    } finally {
      pending.value = false
    }
  }

  return {
    summary,
    pending,
    error,
    load,
  }
})
