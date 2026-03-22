import { defineStore } from 'pinia'
import {
  fetchExtensionInstances,
  fetchSystemSummary,
  type FetchExtensionInstancesOptions,
  type ExtensionInstancesResponse,
  type SystemSummaryResponse,
} from '~/entities/system/api'

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

  const syncExtensionInstances = (payload: ExtensionInstancesResponse) => {
    if (!summary.value) {
      return
    }
    summary.value.single_tab_extension.instances = payload.items
    summary.value.single_tab_extension.active_sessions = payload.summary.active_sessions
    summary.value.single_tab_extension.host_disconnected_sessions = payload.summary.host_disconnected_sessions
    summary.value.single_tab_extension.transport_mode = payload.meta.transport_mode
    summary.value.single_tab_extension.relay_enabled = payload.meta.relay_enabled
    summary.value.single_tab_extension.relay_heartbeat_ttl_seconds = payload.meta.relay_heartbeat_ttl_seconds
  }

  const refreshExtensionInstances = async (options?: FetchExtensionInstancesOptions): Promise<ExtensionInstancesResponse | null> => {
    if (!summary.value || !summary.value.single_tab_extension.relay_enabled) {
      return null
    }
    try {
      const payload = await fetchExtensionInstances(options)
      syncExtensionInstances(payload)
      return payload
    } catch {
      // Keep the latest known snapshot and avoid surfacing background polling errors globally.
      return null
    }
  }

  return {
    summary,
    pending,
    error,
    load,
    refreshExtensionInstances,
  }
})
