import { apiRequest } from '@shared/api/client'

export interface SystemHealth {
  status: string
  degraded_components: string[]
}

export interface SystemDoctor {
  status: string
  checked_at: string
  stale: boolean
}

export interface SystemProvider {
  name: string
  active: boolean
  configured: boolean
}

export interface SystemQueue {
  enabled: boolean
  status: string
}

export interface SystemFailure {
  run_id: string
  status: string
  error: string
  updated_at: string
}

export interface SystemPartialError {
  source: string
  error: string
}

export interface SingleTabExtensionSummary {
  transport_mode: 'native_only' | 'dual' | 'remote_preferred' | string
  relay_enabled: boolean
  extension_auth_configured: boolean
  relay_heartbeat_ttl_seconds: number
  active_sessions: number
  host_disconnected_sessions: number
  instances: SingleTabExtensionInstance[]
}

export interface SingleTabExtensionInstance {
  browser_instance_id: string
  last_seen_at: string
  active_sessions: number
  host_disconnected_sessions: number
  state: 'online' | 'stale' | 'disconnected' | 'unknown' | string
}

export interface SystemSummaryResponse {
  health: SystemHealth
  doctor: SystemDoctor
  providers: SystemProvider[]
  queues: {
    memory_pipeline: SystemQueue
  }
  pending_approvals: number
  recent_failures: SystemFailure[]
  degraded_components: string[]
  partial_errors: SystemPartialError[]
  single_tab_extension: SingleTabExtensionSummary
}

export const fetchSystemSummary = () => {
  return apiRequest<SystemSummaryResponse>('/api/v2/system')
}

export interface ExtensionInstancesSummary {
  instances_total: number
  instances_matched: number
  online: number
  stale: number
  disconnected: number
  unknown: number
  active_sessions: number
  host_disconnected_sessions: number
  truncated: boolean
  captured_at: string
}

export interface ExtensionInstancesMeta {
  limit: number
  state_filter: string[]
  transport_mode: 'native_only' | 'dual' | 'remote_preferred' | string
  relay_enabled: boolean
  relay_heartbeat_ttl_seconds: number
}

export interface ExtensionInstancesLivenessPolicy {
  heartbeat_ttl_seconds: number
  online_when_last_seen_within_ttl: boolean
  stale_when_last_seen_exceeds_ttl: boolean
  disconnected_when_only_disconnected_sessions: boolean
  unknown_when_last_seen_missing: boolean
}

export interface ExtensionInstancesResponse {
  items: SingleTabExtensionInstance[]
  summary: ExtensionInstancesSummary
  meta: ExtensionInstancesMeta
  liveness_policy: ExtensionInstancesLivenessPolicy
  partial_errors: SystemPartialError[]
}

export interface FetchExtensionInstancesOptions {
  limit?: number
  state?: string[]
}

export const fetchExtensionInstances = (options?: FetchExtensionInstancesOptions) => {
  const query = new URLSearchParams()
  if (options?.limit && options.limit > 0) {
    query.set('limit', String(options.limit))
  }
  if (options?.state && options.state.length > 0) {
    query.set('state', options.state.join(','))
  }
  const suffix = query.toString()
  return apiRequest<ExtensionInstancesResponse>(
    suffix ? `/api/v2/single-tab/extension-instances?${suffix}` : '/api/v2/single-tab/extension-instances'
  )
}
