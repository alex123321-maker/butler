<template>
  <main class="page">
    <h2 class="page-title">System</h2>

    <AppAlert v-if="error" tone="error">Unable to load system summary.</AppAlert>
    <p v-else-if="pending" class="placeholder-text">Loading system summary...</p>

    <AppEmptyState
      v-else-if="!systemData"
      title="System summary is unavailable"
      description="Try refreshing the page in a few seconds."
    />

    <template v-else>
      <AppPanel title="Health">
        <div class="system-grid system-grid--health">
          <div>
            <p class="system-label">Status</p>
            <AppBadge :tone="healthTone(systemData.health.status)">{{ systemData.health.status }}</AppBadge>
          </div>
          <div>
            <p class="system-label">Pending approvals</p>
            <p class="system-value">{{ systemData.pending_approvals }}</p>
          </div>
          <div>
            <p class="system-label">Doctor</p>
            <p class="system-value">
              {{ systemData.doctor.status || 'unknown' }}
              <span v-if="systemData.doctor.stale"> (stale)</span>
            </p>
          </div>
        </div>
      </AppPanel>

      <div class="system-section">
        <AppPanel title="Degraded zones">
          <AppEmptyState
            v-if="systemData.degraded_components.length === 0"
            title="No degraded components"
            description="All monitored components are healthy."
          />
          <ul v-else class="system-list">
            <li v-for="component in systemData.degraded_components" :key="component">
              <AppBadge tone="warning">{{ component }}</AppBadge>
            </li>
          </ul>
        </AppPanel>
      </div>

      <div class="system-section">
        <AppPanel title="Active warnings">
          <AppEmptyState
            v-if="warnings.length === 0"
            title="No active warnings"
            description="No immediate operator action is required."
          />
          <ul v-else class="warning-list">
            <li v-for="warning in warnings" :key="warning" class="warning-item">{{ warning }}</li>
          </ul>
        </AppPanel>
      </div>

      <div class="system-section">
        <AppPanel title="Providers">
          <AppTable>
            <thead>
              <tr>
                <th>Name</th>
                <th>Active</th>
                <th>Configured</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="provider in systemData.providers" :key="provider.name">
                <td>{{ provider.name }}</td>
                <td>{{ provider.active ? 'yes' : 'no' }}</td>
                <td>{{ provider.configured ? 'yes' : 'no' }}</td>
              </tr>
            </tbody>
          </AppTable>
        </AppPanel>
      </div>

      <div class="system-section">
        <AppPanel title="Recent failures">
          <AppEmptyState
            v-if="systemData.recent_failures.length === 0"
            title="No recent failures"
            description="No failed tasks were detected in latest window."
          />
          <AppTable v-else>
            <thead>
              <tr>
                <th>Run</th>
                <th>Error</th>
                <th>Updated</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="failure in systemData.recent_failures" :key="failure.run_id">
                <td>{{ failure.run_id }}</td>
                <td>{{ failure.error || '-' }}</td>
                <td>{{ formatDate(failure.updated_at) }}</td>
              </tr>
            </tbody>
          </AppTable>
        </AppPanel>
      </div>
    </template>
  </main>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import AppAlert from '~/shared/ui/AppAlert.vue'
import AppBadge from '~/shared/ui/AppBadge.vue'
import AppEmptyState from '~/shared/ui/AppEmptyState.vue'
import AppPanel from '~/shared/ui/AppPanel.vue'
import AppTable from '~/shared/ui/AppTable.vue'
import { useSystemStore } from '~/shared/model/stores/system'

useHead({ title: 'System - Butler' })

const systemStore = useSystemStore()
const { summary: systemData, pending, error } = storeToRefs(systemStore)

const healthTone = (status: string): 'default' | 'success' | 'warning' | 'error' | 'info' => {
  if (status === 'healthy') {
    return 'success'
  }
  if (status === 'degraded') {
    return 'warning'
  }
  return 'default'
}

const warnings = computed(() => {
  if (!systemData.value) {
    return [] as string[]
  }

  const items: string[] = []
  if (systemData.value.pending_approvals > 0) {
    items.push(`Pending approvals: ${systemData.value.pending_approvals}`)
  }
  if (systemData.value.recent_failures.length > 0) {
    items.push(`Recent failed tasks: ${systemData.value.recent_failures.length}`)
  }
  if (systemData.value.doctor.stale) {
    items.push('Doctor report is stale and should be refreshed')
  }
  for (const partial of systemData.value.partial_errors) {
    items.push(`${partial.source}: ${partial.error}`)
  }
  return items
})

const formatDate = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return date.toLocaleString()
}

onMounted(async () => {
  await systemStore.load()
})
</script>

<style scoped>
.system-section {
  margin-top: var(--space-4);
}

.system-grid {
  display: grid;
  gap: var(--space-4);
}

.system-grid--health {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.system-label {
  margin: 0 0 var(--space-1);
  color: var(--color-text-secondary);
  font-size: 12px;
}

.system-value {
  margin: 0;
}

.system-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.warning-list {
  margin: 0;
  padding-left: var(--space-4);
}

.warning-item {
  margin-bottom: var(--space-2);
}

@media (max-width: 900px) {
  .system-grid--health {
    grid-template-columns: 1fr;
  }
}
</style>
