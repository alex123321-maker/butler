<template>
  <NuxtPage v-if="hasTaskDetailRoute" />

  <main v-else class="page">
    <h2 class="page-title">Tasks</h2>
    <div class="task-filters">
      <label>
        Status
        <select v-model="filters.status">
          <option value="">all</option>
          <option value="in_progress">in_progress</option>
          <option value="waiting_for_approval">waiting_for_approval</option>
          <option value="waiting_for_reply_in_telegram">waiting_for_reply_in_telegram</option>
          <option value="completed">completed</option>
          <option value="failed">failed</option>
          <option value="cancelled">cancelled</option>
          <option value="completed_with_issues">completed_with_issues</option>
        </select>
      </label>

      <label>
        Needs user action
        <select v-model="needsActionRaw">
          <option value="">all</option>
          <option value="true">true</option>
          <option value="false">false</option>
        </select>
      </label>

      <label>
        Waiting reason
        <input v-model="filters.waitingReason" type="text" placeholder="approval_required" />
      </label>

      <label>
        Source channel
        <input v-model="filters.sourceChannel" type="text" placeholder="telegram" />
      </label>

      <label>
        Provider
        <input v-model="filters.provider" type="text" placeholder="openai" />
      </label>

      <label>
        From
        <input v-model="filters.from" type="datetime-local" />
      </label>

      <label>
        To
        <input v-model="filters.to" type="datetime-local" />
      </label>

      <label>
        Search
        <input v-model="filters.query" type="text" placeholder="run id, session key, summary" />
      </label>

      <label>
        Sort
        <select v-model="filters.sort">
          <option value="-updated_at">updated_at desc</option>
          <option value="updated_at">updated_at asc</option>
          <option value="-started_at">started_at desc</option>
          <option value="started_at">started_at asc</option>
        </select>
      </label>

      <label>
        Page size
        <select v-model.number="filters.limit">
          <option :value="20">20</option>
          <option :value="50">50</option>
          <option :value="100">100</option>
        </select>
      </label>

      <div class="task-filters__actions">
        <button class="btn" type="button" :disabled="pending" @click="applyFilters">Apply</button>
        <button class="btn" type="button" :disabled="pending" @click="resetFilters">Reset</button>
      </div>
    </div>

    <p class="task-meta">Total: {{ total }}<span v-if="pending"> · loading...</span></p>

    <div class="task-pagination">
      <button class="btn" type="button" :disabled="pending || filters.offset <= 0" @click="prevPage">Prev</button>
      <span>Offset: {{ filters.offset }}</span>
      <button class="btn" type="button" :disabled="pending || !canNext" @click="nextPage">Next</button>
    </div>

    <AppAlert v-if="error" tone="error">Unable to load tasks list.</AppAlert>
    <p v-else-if="pending" class="placeholder-text">Loading tasks...</p>
    <p v-else-if="tasks.length === 0" class="placeholder-text">No tasks found for current filters.</p>
    <AppTable v-else>
      <thead>
        <tr>
          <th>Task ID</th>
          <th>Status</th>
          <th>Waiting Reason</th>
          <th>Action Channel</th>
          <th>Updated</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="task in tasks" :key="task.task_id">
          <td>
            <NuxtLink :to="`/tasks/${task.task_id}`">{{ task.task_id }}</NuxtLink>
          </td>
          <td>{{ task.status }}</td>
          <td>{{ task.waiting_reason || '-' }}</td>
          <td>{{ task.user_action_channel || '-' }}</td>
          <td>{{ task.updated_at }}</td>
        </tr>
      </tbody>
    </AppTable>
  </main>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import AppAlert from '~/shared/ui/AppAlert.vue'
import AppTable from '~/shared/ui/AppTable.vue'
import { useTasksStore } from '~/shared/model/stores/tasks'

useHead({ title: 'Tasks - Butler' })

const tasksStore = useTasksStore()
const { items: tasks, total, error, pending, filters } = storeToRefs(tasksStore)
const route = useRoute()

const hasTaskDetailRoute = computed(() => {
  const value = route.params.id
  if (Array.isArray(value)) {
    return value.length > 0
  }
  return typeof value === 'string' && value.length > 0
})

const needsActionRaw = computed({
  get: () => {
    if (filters.value.needsUserAction === null) {
      return ''
    }
    return String(filters.value.needsUserAction)
  },
  set: (value: string) => {
    if (value === '') {
      filters.value.needsUserAction = null
      return
    }
    filters.value.needsUserAction = value === 'true'
  },
})

const applyFilters = async () => {
  await tasksStore.load({
    status: filters.value.status,
    needsUserAction: filters.value.needsUserAction,
    waitingReason: filters.value.waitingReason,
    sourceChannel: filters.value.sourceChannel,
    provider: filters.value.provider,
    from: filters.value.from,
    to: filters.value.to,
    query: filters.value.query,
    sort: filters.value.sort,
    limit: filters.value.limit,
    offset: 0,
  })
}

const resetFilters = async () => {
  filters.value.status = ''
  filters.value.needsUserAction = null
  filters.value.waitingReason = ''
  filters.value.sourceChannel = ''
  filters.value.provider = ''
  filters.value.from = ''
  filters.value.to = ''
  filters.value.query = ''
  filters.value.sort = '-updated_at'
  filters.value.limit = 50
  await tasksStore.load({ offset: 0 })
}

const canNext = computed(() => {
  return filters.value.offset + tasks.value.length < total.value
})

const prevPage = async () => {
  const nextOffset = Math.max(0, filters.value.offset - filters.value.limit)
  await tasksStore.load({ offset: nextOffset })
}

const nextPage = async () => {
  await tasksStore.load({ offset: filters.value.offset + filters.value.limit })
}

onMounted(async () => {
  if (hasTaskDetailRoute.value) {
    return
  }

  await tasksStore.load({ offset: 0 })
})
</script>

<style scoped>
.task-filters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
  margin-bottom: var(--space-4);
}

.task-filters label {
  display: grid;
  gap: var(--space-1);
  color: var(--color-text-secondary);
  font-size: 13px;
}

.task-filters input,
.task-filters select {
  background: var(--color-bg-surfaceMuted);
  color: var(--color-text-primary);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  padding: var(--space-2) var(--space-3);
}

.task-filters__actions {
  display: flex;
  gap: var(--space-2);
  align-items: end;
}

.task-meta {
  color: var(--color-text-secondary);
  margin-bottom: var(--space-3);
}

.task-pagination {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-3);
  color: var(--color-text-secondary);
}
</style>
