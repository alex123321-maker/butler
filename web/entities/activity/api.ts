import { apiRequest } from '@shared/api/client'

export interface ActivityEvent {
  activity_id: number
  run_id: string
  session_key: string
  activity_type: string
  summary: string
  details_json: string
  actor_type: string
  severity: string
  title: string
  created_at: string
}

export interface ActivityListResponse {
  activity: ActivityEvent[]
}

export const fetchActivity = (query: Record<string, string | number | undefined> = {}) => {
  return apiRequest<ActivityListResponse>('/api/v2/activity', { query })
}
