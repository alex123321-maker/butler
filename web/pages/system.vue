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
        <AppPanel title="Browser Extension Remote">
          <div class="system-grid system-grid--extension">
            <div>
              <p class="system-label">Transport mode</p>
              <AppBadge :tone="extensionModeTone(systemData.single_tab_extension.transport_mode)">
                {{ systemData.single_tab_extension.transport_mode }}
              </AppBadge>
            </div>
            <div>
              <p class="system-label">Extension auth</p>
              <AppBadge :tone="systemData.single_tab_extension.extension_auth_configured ? 'success' : 'warning'">
                {{ systemData.single_tab_extension.extension_auth_configured ? 'configured' : 'missing' }}
              </AppBadge>
            </div>
            <div>
              <p class="system-label">Relay heartbeat TTL</p>
              <p class="system-value">{{ systemData.single_tab_extension.relay_heartbeat_ttl_seconds }}s</p>
            </div>
            <div>
              <p class="system-label">Active single-tab sessions</p>
              <p class="system-value">{{ systemData.single_tab_extension.active_sessions }}</p>
            </div>
            <div>
              <p class="system-label">Host disconnected sessions</p>
              <p class="system-value">{{ systemData.single_tab_extension.host_disconnected_sessions }}</p>
            </div>
          </div>

          <AppAlert v-if="!systemData.single_tab_extension.relay_enabled" tone="warning">
            Remote extension monitoring is disabled in `native_only` transport mode.
          </AppAlert>

          <div v-else class="extension-toolbar">
            <div class="extension-toolbar__chips">
              <button
                v-for="chip in extensionStateChips"
                :key="chip.key"
                type="button"
                class="filter-chip"
                :class="{ 'filter-chip--active': extensionStateFilter[chip.key] }"
                @click="toggleExtensionStateFilter(chip.key)"
              >
                {{ chip.label }}
              </button>
            </div>

            <div class="extension-toolbar__controls">
              <label class="extension-toolbar__limit">
                <span class="system-label">Rows</span>
                <AppSelect v-model.number="extensionLimit" @change="applyExtensionLimit">
                  <option :value="25">25</option>
                  <option :value="50">50</option>
                  <option :value="100">100</option>
                  <option :value="200">200</option>
                </AppSelect>
              </label>
              <AppButton :disabled="extensionRefreshPending" @click="refreshExtensionSnapshot">Refresh</AppButton>
              <AppButton :disabled="extensionRefreshPending" @click="resetExtensionFilters">Reset</AppButton>
            </div>

            <div class="extension-toolbar__stats">
              <span class="stat-pill">{{ extensionShownCount }} shown</span>
              <span class="stat-pill">{{ extensionMatchedCount }} matched</span>
              <span class="stat-pill">{{ extensionTotalCount }} total</span>
              <span v-if="extensionInstancesTruncated" class="stat-pill stat-pill--warning">truncated by limit</span>
              <span class="stat-pill">last refresh: {{ extensionLastCapturedAt ? formatDate(extensionLastCapturedAt) : '-' }}</span>
            </div>
          </div>

          <AppEmptyState
            v-if="systemData.single_tab_extension.instances.length === 0"
            title="No extension instances yet"
            description="Connect extension in remote mode and start a single-tab session to see live instance status."
          />
          <AppTable v-else>
            <thead>
              <tr>
                <th>Browser instance</th>
                <th>State</th>
                <th>Last heartbeat</th>
                <th>Active sessions</th>
                <th>Disconnected sessions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="instance in systemData.single_tab_extension.instances" :key="instance.browser_instance_id">
                <td>{{ instance.browser_instance_id }}</td>
                <td>
                  <AppBadge :tone="extensionInstanceTone(instance.state)">
                    {{ instance.state }}
                  </AppBadge>
                </td>
                <td>{{ instance.last_seen_at ? formatDate(instance.last_seen_at) : '-' }}</td>
                <td>{{ instance.active_sessions }}</td>
                <td>{{ instance.host_disconnected_sessions }}</td>
              </tr>
            </tbody>
          </AppTable>

          <ul class="warning-list system-list--extension">
            <li>Set `BUTLER_SINGLE_TAB_TRANSPORT_MODE=remote_preferred` for remote-first rollout.</li>
            <li>Configure `BUTLER_EXTENSION_API_TOKENS` before connecting extension to remote API.</li>
            <li>Use extension popup with `Rollout mode = remote_preferred` for non-localhost deployment.</li>
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
import AppButton from '~/shared/ui/AppButton.vue'
import AppEmptyState from '~/shared/ui/AppEmptyState.vue'
import AppPanel from '~/shared/ui/AppPanel.vue'
import AppSelect from '~/shared/ui/AppSelect.vue'
import AppTable from '~/shared/ui/AppTable.vue'
import { useSystemStore } from '~/shared/model/stores/system'

useHead({ title: 'System - Butler' })

const systemStore = useSystemStore()
const { summary: systemData, pending, error } = storeToRefs(systemStore)
const extensionPollIntervalMs = 7000
let extensionPollTimer: ReturnType<typeof setInterval> | null = null
const extensionLimit = ref(50)
const extensionRefreshPending = ref(false)
const extensionLastCapturedAt = ref('')
const extensionInstancesTotal = ref<number | null>(null)
const extensionInstancesMatched = ref<number | null>(null)
const extensionInstancesTruncated = ref(false)
const extensionPartialWarnings = ref<string[]>([])
const extensionStateFilter = ref({
  online: true,
  stale: true,
  disconnected: true,
  unknown: true,
})
const extensionStateChips = [
  { key: 'online', label: 'Online' },
  { key: 'stale', label: 'Stale' },
  { key: 'disconnected', label: 'Disconnected' },
  { key: 'unknown', label: 'Unknown' },
] as const

type ExtensionStateKey = (typeof extensionStateChips)[number]['key']

const healthTone = (status: string): 'default' | 'success' | 'warning' | 'error' | 'info' => {
  if (status === 'healthy') {
    return 'success'
  }
  if (status === 'degraded') {
    return 'warning'
  }
  return 'default'
}

const extensionModeTone = (mode: string): 'default' | 'success' | 'warning' | 'error' | 'info' => {
  if (mode === 'remote_preferred') {
    return 'success'
  }
  if (mode === 'native_only') {
    return 'warning'
  }
  return 'info'
}

const extensionInstanceTone = (state: string): 'default' | 'success' | 'warning' | 'error' | 'info' => {
  if (state === 'online') {
    return 'success'
  }
  if (state === 'stale') {
    return 'warning'
  }
  if (state === 'disconnected') {
    return 'error'
  }
  return 'default'
}

const selectedExtensionStates = computed(() => {
  const selected: string[] = []
  for (const chip of extensionStateChips) {
    if (extensionStateFilter.value[chip.key]) {
      selected.push(chip.key)
    }
  }
  return selected
})

const extensionShownCount = computed(() => {
  return systemData.value?.single_tab_extension.instances.length ?? 0
})

const extensionMatchedCount = computed(() => {
  return extensionInstancesMatched.value ?? extensionShownCount.value
})

const extensionTotalCount = computed(() => {
  return extensionInstancesTotal.value ?? extensionShownCount.value
})

const extensionRemoteMonitoringEnabled = computed(() => {
  return systemData.value?.single_tab_extension.relay_enabled === true
})

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
  if (!systemData.value.single_tab_extension.extension_auth_configured) {
    items.push('Single-tab extension auth tokens are not configured')
  }
  if (systemData.value.single_tab_extension.host_disconnected_sessions > 0) {
    items.push(`Host disconnected single-tab sessions: ${systemData.value.single_tab_extension.host_disconnected_sessions}`)
  }
  const staleInstances = systemData.value.single_tab_extension.instances.filter((instance) => instance.state === 'stale').length
  if (staleInstances > 0) {
    items.push(`Stale extension instances: ${staleInstances}`)
  }
  for (const extensionWarning of extensionPartialWarnings.value) {
    items.push(`extension_instances: ${extensionWarning}`)
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

const refreshExtensionSnapshot = async () => {
  if (!extensionRemoteMonitoringEnabled.value) {
    extensionPartialWarnings.value = []
    extensionLastCapturedAt.value = ''
    extensionInstancesTruncated.value = false
    return
  }
  extensionRefreshPending.value = true
  const payload = await systemStore.refreshExtensionInstances({
    limit: extensionLimit.value,
    state: selectedExtensionStates.value,
  })
  if (payload) {
    extensionLastCapturedAt.value = payload.summary.captured_at
    extensionInstancesTotal.value = payload.summary.instances_total
    extensionInstancesMatched.value = payload.summary.instances_matched
    extensionInstancesTruncated.value = payload.summary.truncated
    extensionPartialWarnings.value = payload.partial_errors.map((partial) => `${partial.source}: ${partial.error}`)
  }
  extensionRefreshPending.value = false
}

const toggleExtensionStateFilter = async (key: ExtensionStateKey) => {
  extensionStateFilter.value[key] = !extensionStateFilter.value[key]
  await refreshExtensionSnapshot()
}

const applyExtensionLimit = async () => {
  await refreshExtensionSnapshot()
}

const resetExtensionFilters = async () => {
  extensionLimit.value = 50
  extensionStateFilter.value.online = true
  extensionStateFilter.value.stale = true
  extensionStateFilter.value.disconnected = true
  extensionStateFilter.value.unknown = true
  await refreshExtensionSnapshot()
}

onMounted(async () => {
  await systemStore.load()
  extensionInstancesTotal.value = systemData.value?.single_tab_extension.instances.length ?? 0
  extensionInstancesMatched.value = systemData.value?.single_tab_extension.instances.length ?? 0
  await refreshExtensionSnapshot()
  if (extensionPollTimer !== null) {
    clearInterval(extensionPollTimer)
  }
  extensionPollTimer = setInterval(() => {
    void refreshExtensionSnapshot()
  }, extensionPollIntervalMs)
})

onUnmounted(() => {
  if (extensionPollTimer === null) {
    return
  }
  clearInterval(extensionPollTimer)
  extensionPollTimer = null
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

.system-grid--extension {
  grid-template-columns: repeat(5, minmax(0, 1fr));
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

.extension-toolbar {
  margin-top: var(--space-4);
  margin-bottom: var(--space-4);
  padding: var(--space-3);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  display: grid;
  gap: var(--space-3);
  background: var(--color-bg-surfaceMuted);
}

.extension-toolbar__chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.filter-chip {
  border: 1px solid var(--color-border-default);
  background: var(--color-bg-surface);
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

.extension-toolbar__controls {
  display: flex;
  align-items: flex-end;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.extension-toolbar__limit {
  width: 120px;
  display: grid;
  gap: var(--space-1);
}

.extension-toolbar__stats {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.stat-pill {
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-full);
  padding: var(--space-1) var(--space-3);
  color: var(--color-text-secondary);
  font-size: 12px;
  background: var(--color-bg-surface);
}

.stat-pill--warning {
  color: var(--color-state-warning);
}

.warning-item {
  margin-bottom: var(--space-2);
}

.system-list--extension {
  margin-top: var(--space-4);
}

@media (max-width: 900px) {
  .system-grid--health {
    grid-template-columns: 1fr;
  }

  .system-grid--extension {
    grid-template-columns: 1fr;
  }
}
</style>
