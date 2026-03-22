<template>
  <NuxtPage v-if="hasTaskDetailRoute" />

  <main v-else class="page">
    <section class="page-header">
      <div class="page-header__content">
        <p class="page-kicker">Workspace</p>
        <h2 class="page-title">Tasks</h2>
        <p class="page-copy">
          Follow what Butler is doing right now, what is blocked, and where you need to step in.
          Technical filters are still available, but they no longer dominate the main view.
        </p>
      </div>
      <div class="page-header__actions">
        <AppButton variant="secondary" :disabled="pending" @click="showAdvanced = !showAdvanced">
          {{ showAdvanced ? 'Hide filters' : 'Advanced filters' }}
        </AppButton>
        <AppButton variant="secondary" :disabled="pending" @click="resetFilters">Reset</AppButton>
      </div>
    </section>

    <section class="page-section">
      <div class="task-toolbar">
        <label class="task-search">
          <span class="task-search__label">Search</span>
          <AppInput
            v-model="filters.query"
            type="text"
            placeholder="Find by task id, summary, or session context"
            @keyup.enter="applyFilters"
          />
        </label>

        <div class="task-quick-filters">
          <button
            v-for="chip in quickStatusChips"
            :key="chip.value"
            type="button"
            class="filter-chip"
            :class="{ 'filter-chip--active': filters.status === chip.value }"
            @click="selectStatus(chip.value)"
          >
            {{ chip.label }}
          </button>
        </div>

        <label class="task-toggle">
          <input v-model="needsAttentionOnly" type="checkbox" @change="applyFilters">
          <span>Only tasks waiting on me</span>
        </label>
      </div>

      <div v-if="showAdvanced" class="advanced-filters">
        <label>
          Source channel
          <AppInput v-model="filters.sourceChannel" type="text" placeholder="telegram" />
        </label>
        <label>
          Provider
          <AppInput v-model="filters.provider" type="text" placeholder="openai" />
        </label>
        <label>
          Waiting reason
          <AppInput v-model="filters.waitingReason" type="text" placeholder="approval_required" />
        </label>
        <label>
          Sort
          <AppSelect v-model="filters.sort">
            <option value="-updated_at">Most recently updated</option>
            <option value="updated_at">Oldest updated first</option>
            <option value="-started_at">Newest started first</option>
            <option value="started_at">Oldest started first</option>
          </AppSelect>
        </label>
        <label>
          From
          <AppInput v-model="filters.from" type="datetime-local" />
        </label>
        <label>
          To
          <AppInput v-model="filters.to" type="datetime-local" />
        </label>
        <label>
          Page size
          <AppSelect v-model.number="filters.limit">
            <option :value="20">20</option>
            <option :value="50">50</option>
            <option :value="100">100</option>
          </AppSelect>
        </label>
        <div class="advanced-filters__actions">
          <AppButton variant="primary" :disabled="pending" @click="applyFilters">Apply filters</AppButton>
        </div>
      </div>

      <div class="task-meta-row">
        <div class="pill-row">
          <span class="pill">{{ total }} total</span>
          <span class="pill">{{ tasks.length }} on this page</span>
          <span class="pill" v-if="filters.status">{{ formatTaskStatus(filters.status) }}</span>
          <span class="pill" v-if="needsAttentionOnly">Needs my action</span>
        </div>
        <div class="task-pagination">
          <AppButton variant="secondary" :disabled="pending || filters.offset <= 0" @click="prevPage">Previous</AppButton>
          <span class="pagination-copy">Offset {{ filters.offset }}</span>
          <AppButton variant="secondary" :disabled="pending || !canNext" @click="nextPage">Next</AppButton>
        </div>
      </div>
    </section>

    <AppAlert v-if="error" tone="error">Unable to load tasks list.</AppAlert>
    <p v-else-if="pending" class="placeholder-text">Loading tasks…</p>
    <AppEmptyState
      v-else-if="tasks.length === 0"
      title="No tasks match the current view"
      description="Try resetting filters or switching back to all tasks."
    />

    <section v-else class="task-list">
      <NuxtLink
        v-for="task in tasks"
        :key="task.task_id"
        :to="`/tasks/${task.task_id}`"
        class="task-card"
      >
        <div class="task-card__topline">
          <div class="task-card__titleblock">
            <p class="task-card__title">{{ formatTaskId(task.task_id) }}</p>
            <p class="task-card__summary">{{ describeTaskState(task) }}</p>
          </div>
          <AppBadge :tone="taskTone(task.status)">{{ formatTaskStatus(task.status) }}</AppBadge>
        </div>

        <div class="task-card__meta">
          <span class="pill">{{ formatWaitingReason(task.waiting_reason) }}</span>
          <span class="pill" v-if="task.needs_user_action">Needs your action</span>
          <span class="pill" v-if="task.user_action_channel">{{ formatChannel(task.user_action_channel) }}</span>
          <span class="pill" v-if="task.source_channel">{{ formatChannel(task.source_channel) }}</span>
        </div>

        <div class="task-card__footer">
          <span>Updated {{ formatDateTime(task.updated_at) }}</span>
          <span class="text-link">Open details</span>
        </div>
      </NuxtLink>
    </section>
  </main>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import AppAlert from '~/shared/ui/AppAlert.vue'
import AppBadge from '~/shared/ui/AppBadge.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppEmptyState from '~/shared/ui/AppEmptyState.vue'
import AppInput from '~/shared/ui/AppInput.vue'
import AppSelect from '~/shared/ui/AppSelect.vue'
import {
  describeTaskState,
  formatChannel,
  formatDateTime,
  formatTaskId,
  formatTaskStatus,
  formatWaitingReason,
  taskTone,
} from '~/entities/task/presentation'
import { useTasksStore } from '~/shared/model/stores/tasks'

useHead({ title: 'Tasks - Butler' })

const tasksStore = useTasksStore()
const { items: tasks, total, error, pending, filters } = storeToRefs(tasksStore)
const route = useRoute()

const showAdvanced = ref(false)

const quickStatusChips = [
  { value: '', label: 'All tasks' },
  { value: 'in_progress', label: 'In progress' },
  { value: 'waiting_for_approval', label: 'Waiting approval' },
  { value: 'waiting_for_reply_in_telegram', label: 'Waiting on me' },
  { value: 'completed', label: 'Completed' },
  { value: 'failed', label: 'Failed' },
]

const hasTaskDetailRoute = computed(() => {
  const value = route.params.id
  if (Array.isArray(value)) {
    return value.length > 0
  }
  return typeof value === 'string' && value.length > 0
})

const needsAttentionOnly = computed({
  get: () => filters.value.needsUserAction === true,
  set: (value: boolean) => {
    filters.value.needsUserAction = value ? true : null
  },
})

async function selectStatus(status: string) {
  filters.value.status = status
  await applyFilters()
}

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
  showAdvanced.value = false
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
.task-toolbar {
  display: grid;
  gap: var(--space-4);
}

.task-search {
  display: grid;
  gap: var(--space-2);
}

.task-search__label {
  color: var(--color-text-secondary);
  font-size: var(--text-sm);
}

.task-quick-filters {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.filter-chip {
  border: 1px solid var(--color-border-default);
  background: var(--color-bg-surfaceMuted);
  color: var(--color-text-secondary);
  border-radius: var(--radius-full);
  padding: var(--space-2) var(--space-3);
  cursor: pointer;
}

.filter-chip--active {
  background: var(--color-accent-primaryMuted);
  border-color: var(--color-accent-primary);
  color: var(--color-text-primary);
}

.task-toggle {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--color-text-secondary);
}

.advanced-filters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
  padding-top: var(--space-4);
  border-top: 1px solid var(--color-border-default);
}

.advanced-filters label {
  display: grid;
  gap: var(--space-2);
  color: var(--color-text-secondary);
  font-size: var(--text-sm);
}

.advanced-filters__actions {
  display: flex;
  align-items: flex-end;
}

.task-meta-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  flex-wrap: wrap;
}

.task-pagination {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.pagination-copy {
  color: var(--color-text-secondary);
  font-size: var(--text-sm);
}

.task-list {
  display: grid;
  gap: var(--space-3);
}

.task-card {
  display: grid;
  gap: var(--space-4);
  padding: var(--space-5);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
  color: inherit;
  transition: border-color var(--transition-normal), background-color var(--transition-normal);
}

.task-card:hover {
  background: var(--color-bg-elevated);
  border-color: var(--color-border-strong);
}

.task-card__topline {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}

.task-card__titleblock {
  display: grid;
  gap: var(--space-2);
}

.task-card__title {
  margin: 0;
  font-size: var(--text-lg);
  font-weight: var(--font-semibold);
}

.task-card__summary {
  margin: 0;
  color: var(--color-text-secondary);
}

.task-card__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.task-card__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  color: var(--color-text-secondary);
  font-size: var(--text-sm);
}

@media (max-width: 720px) {
  .task-card__topline,
  .task-card__footer {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
