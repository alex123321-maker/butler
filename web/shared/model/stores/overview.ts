import { defineStore } from 'pinia'
import { fetchOverview, type OverviewResponse } from '~/entities/overview/api'

interface OverviewCounts {
  attention_items_count: number
  active_tasks_count: number
  approvals_pending_count: number
  failed_tasks_count: number
}

const defaultCounts = (): OverviewCounts => ({
  attention_items_count: 0,
  active_tasks_count: 0,
  approvals_pending_count: 0,
  failed_tasks_count: 0,
})

export const useOverviewStore = defineStore('overview', () => {
  const data = ref<OverviewResponse | null>(null)
  const counts = ref<OverviewCounts>(defaultCounts())
  const pending = ref(false)
  const error = ref<string | null>(null)

  const load = async () => {
    pending.value = true
    error.value = null

    try {
      const response = await fetchOverview()
      data.value = response
      counts.value = {
        ...defaultCounts(),
        ...(response.counts ?? {}),
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load overview'
    } finally {
      pending.value = false
    }
  }

  return {
    data,
    counts,
    pending,
    error,
    load,
  }
})
