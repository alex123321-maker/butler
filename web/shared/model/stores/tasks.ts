import { defineStore } from 'pinia'
import { fetchTasks, type TaskItem, type TaskListResponse } from '~/entities/task/api'

export interface TaskFilters {
  status: string
  needsUserAction: boolean | null
  waitingReason: string
  sourceChannel: string
  provider: string
  from: string
  to: string
  query: string
  sort: string
  limit: number
  offset: number
}

const defaultFilters = (): TaskFilters => ({
  status: '',
  needsUserAction: null,
  waitingReason: '',
  sourceChannel: '',
  provider: '',
  from: '',
  to: '',
  query: '',
  sort: '-updated_at',
  limit: 50,
  offset: 0,
})

export const useTasksStore = defineStore('tasks', () => {
  const items = ref<TaskItem[]>([])
  const total = ref(0)
  const pending = ref(false)
  const error = ref<string | null>(null)
  const filters = ref<TaskFilters>(defaultFilters())

  const load = async (overrides: Partial<TaskFilters> = {}) => {
    filters.value = {
      ...filters.value,
      ...overrides,
    }

    pending.value = true
    error.value = null

    try {
      const query: Record<string, string | number | boolean | undefined> = {
        status: filters.value.status || undefined,
        needs_user_action: filters.value.needsUserAction ?? undefined,
        waiting_reason: filters.value.waitingReason || undefined,
        source_channel: filters.value.sourceChannel || undefined,
        provider: filters.value.provider || undefined,
        from: filters.value.from || undefined,
        to: filters.value.to || undefined,
        query: filters.value.query || undefined,
        sort: filters.value.sort,
        limit: filters.value.limit,
        offset: filters.value.offset,
      }

      const response: TaskListResponse = await fetchTasks(query)
      items.value = response.items ?? []
      total.value = response.total ?? 0
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load tasks'
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
