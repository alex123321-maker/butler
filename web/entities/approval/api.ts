import { apiRequest } from '@shared/api/client'

export interface ApprovalItem {
  id: string
  run_id: string
  status: string
  tool_name?: string
  summary?: string
  risk_level?: string
  requested_at?: string
}

export interface ApprovalListResponse {
  items: ApprovalItem[]
  total: number
}

export const fetchApprovals = () => {
  return apiRequest<ApprovalListResponse>('/api/v2/approvals')
}

export const approveById = (id: string) => {
  return apiRequest(`/api/v2/approvals/${id}/approve`, { method: 'POST' })
}

export const rejectById = (id: string) => {
  return apiRequest(`/api/v2/approvals/${id}/reject`, { method: 'POST' })
}
