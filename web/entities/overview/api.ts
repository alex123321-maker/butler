import { apiRequest } from '@shared/api/client'

export interface OverviewResponse {
  attention_items: Array<Record<string, unknown>>
  active_tasks: Array<Record<string, unknown>>
  recent_results: Array<Record<string, unknown>>
  system_summary: Record<string, unknown>
  counts: Record<string, number>
}

export const fetchOverview = () => {
  return apiRequest<OverviewResponse>('/api/v2/overview')
}
