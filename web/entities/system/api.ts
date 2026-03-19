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
}

export const fetchSystemSummary = () => {
  return apiRequest<SystemSummaryResponse>('/api/v2/system')
}
