import { defineStore } from 'pinia'
import { fetchActivity, type ActivityEvent, type ActivityListResponse } from '~/entities/activity/api'

export interface ActivityFilters {
  severity: string
  actorType: string
  runID: string
  sessionKey: string
  since: string
  until: string
  limit: number
  offset: number
}

const defaultFilters = (): ActivityFilters => ({
  severity: '',
  actorType: '',
  runID: '',
  sessionKey: '',
  since: '',
  until: '',
  limit: 50,
  offset: 0,
})

export const useActivityStore = defineStore('activity', () => {
  const items = ref<ActivityEvent[]>([])
  const total = ref(0)
  const pending = ref(false)
  const error = ref<string | null>(null)
  const filters = ref<ActivityFilters>(defaultFilters())

  const load = async (overrides: Partial<ActivityFilters> = {}) => {
    filters.value = {
      ...filters.value,
      ...overrides,
    }

    pending.value = true
    error.value = null

    try {
      const query: Record<string, string | number | undefined> = {
        severity: filters.value.severity || undefined,
        actor_type: filters.value.actorType || undefined,
        run_id: filters.value.runID || undefined,
        session_key: filters.value.sessionKey || undefined,
        since: filters.value.since || undefined,
        until: filters.value.until || undefined,
        limit: filters.value.limit,
        offset: filters.value.offset,
      }

      const response: ActivityListResponse = await fetchActivity(query)
      items.value = response.activity ?? []
      total.value = items.value.length
    } catch (err) {
      items.value = []
      total.value = 0
      error.value = err instanceof Error ? err.message : 'Failed to load activity'
    } finally {
      pending.value = false
    }
  }

  return {
    items,
    total,
    pending,
    error,
    filters,
    load,
  }
})
