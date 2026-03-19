<template>
  <main class="page">
    <h2 class="page-title">Activity</h2>

    <div class="activity-filters">
      <label>
        Severity
        <select v-model="filters.severity">
          <option value="">all</option>
          <option value="info">info</option>
          <option value="warning">warning</option>
          <option value="error">error</option>
        </select>
      </label>

      <label>
        Actor
        <input v-model="filters.actorType" type="text" placeholder="system / agent / user" />
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
        Since
        <input v-model="filters.since" type="datetime-local" />
      </label>

      <label>
        Until
        <input v-model="filters.until" type="datetime-local" />
      </label>

      <label>
        Limit
        <select v-model.number="filters.limit">
          <option :value="20">20</option>
          <option :value="50">50</option>
          <option :value="100">100</option>
        </select>
      </label>

      <div class="activity-filters__actions">
        <AppButton variant="primary" :disabled="pending" @click="applyFilters">Apply</AppButton>
        <AppButton :disabled="pending" @click="resetFilters">Reset</AppButton>
      </div>
    </div>

    <p class="activity-meta">Total: {{ total }}<span v-if="pending"> · loading...</span></p>

    <div class="activity-pagination">
      <AppButton :disabled="pending || filters.offset <= 0" @click="prevPage">Prev</AppButton>
      <span>Offset: {{ filters.offset }}</span>
      <AppButton :disabled="pending || !canNext" @click="nextPage">Next</AppButton>
    </div>

    <AppAlert v-if="error" tone="error">Unable to load activity feed.</AppAlert>
    <p v-else-if="pending" class="placeholder-text">Loading activity...</p>
    <AppEmptyState
      v-else-if="events.length === 0"
      title="No activity events"
      description="No events match current filters."
    />

    <AppTable v-else>
      <thead>
        <tr>
          <th>Time</th>
          <th>Type</th>
          <th>Actor</th>
          <th>Run</th>
          <th>Severity</th>
          <th>Event</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="event in events" :key="event.activity_id">
          <td>{{ formatDate(event.created_at) }}</td>
          <td>{{ event.activity_type }}</td>
          <td>{{ event.actor_type || '-' }}</td>
          <td>{{ event.run_id || '-' }}</td>
          <td>{{ event.severity }}</td>
          <td>{{ event.title || event.summary || '-' }}</td>
        </tr>
      </tbody>
    </AppTable>
  </main>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import AppAlert from '~/shared/ui/AppAlert.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppEmptyState from '~/shared/ui/AppEmptyState.vue'
import AppTable from '~/shared/ui/AppTable.vue'
import { useActivityStore } from '~/shared/model/stores/activity'

useHead({ title: 'Activity - Butler' })

const activityStore = useActivityStore()
const { items: events, total, error, pending, filters } = storeToRefs(activityStore)

const canNext = computed(() => {
  return events.value.length >= filters.value.limit
})

const formatDate = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return date.toLocaleString()
}

const applyFilters = async () => {
  await activityStore.load({
    severity: filters.value.severity,
    actorType: filters.value.actorType,
    runID: filters.value.runID,
    sessionKey: filters.value.sessionKey,
    since: filters.value.since,
    until: filters.value.until,
    limit: filters.value.limit,
    offset: 0,
  })
}

const resetFilters = async () => {
  filters.value.severity = ''
  filters.value.actorType = ''
  filters.value.runID = ''
  filters.value.sessionKey = ''
  filters.value.since = ''
  filters.value.until = ''
  filters.value.limit = 50
  await activityStore.load({ offset: 0 })
}

const prevPage = async () => {
  const nextOffset = Math.max(0, filters.value.offset - filters.value.limit)
  await activityStore.load({ offset: nextOffset })
}

const nextPage = async () => {
  await activityStore.load({ offset: filters.value.offset + filters.value.limit })
}

onMounted(async () => {
  await activityStore.load({ offset: 0 })
})
</script>

<style scoped>
.activity-filters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
  margin-bottom: var(--space-4);
}

.activity-filters label {
  display: grid;
  gap: var(--space-1);
  color: var(--color-text-secondary);
  font-size: 13px;
}

.activity-filters input,
.activity-filters select {
  background: var(--color-bg-surfaceMuted);
  color: var(--color-text-primary);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  padding: var(--space-2) var(--space-3);
}

.activity-filters__actions {
  display: flex;
  gap: var(--space-2);
  align-items: end;
}

.activity-meta {
  color: var(--color-text-secondary);
  margin-bottom: var(--space-3);
}

.activity-pagination {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-3);
  color: var(--color-text-secondary);
}
</style>
