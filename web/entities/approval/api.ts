import { apiRequest } from '@shared/api/client'

export interface ApprovalItem {
  id: string
  run_id: string
  status: string
  tool_name?: string
  summary?: string
  risk_level?: string
  requested_at?: string
  args_json?: string
  expires_at?: string
  requested_via?: string
}

export interface ApprovalListResponse {
  items: ApprovalItem[]
  total: number
}

export const fetchApprovals = async (): Promise<ApprovalListResponse> => {
  const response = await apiRequest<{ approvals: Array<Omit<ApprovalItem, 'id'> & { approval_id: string }> }>('/api/v2/approvals')
  const items = (response.approvals ?? []).map(({ approval_id, ...item }) => ({ ...item, id: approval_id }))
  return { items, total: items.length }
}

export const approveById = (id: string, ownerToken: string) => {
  return apiRequest(`/api/v2/approvals/${encodeURIComponent(id)}/approve`, { method: 'POST', headers: { Authorization: `Bearer ${ownerToken}`, 'X-Butler-Delivery-ID': crypto.randomUUID() } })
}

export const rejectById = (id: string, ownerToken: string) => {
  return apiRequest(`/api/v2/approvals/${encodeURIComponent(id)}/reject`, { method: 'POST', headers: { Authorization: `Bearer ${ownerToken}`, 'X-Butler-Delivery-ID': crypto.randomUUID() } })
}

export interface GuardedTabChoices {
  approval_id: string
  state: 'pending' | 'selected'
  expires_at: string
  candidates: Array<{ candidate_token: string; origin: string; ordinal: number; active_at_request: boolean; selected: boolean }>
}

export const fetchGuardedTabChoices = (id: string, ownerToken: string) => apiRequest<GuardedTabChoices>(`/api/v3/tab-choices/${encodeURIComponent(id)}`, { headers: { Authorization: `Bearer ${ownerToken}` } })
export const selectGuardedTab = (id: string, candidateToken: string, ownerToken: string) => apiRequest<{ single_tab_session_id: string }>(`/api/v3/tab-choices/${encodeURIComponent(id)}/select`, { method: 'POST', headers: { Authorization: `Bearer ${ownerToken}`, 'X-Butler-Delivery-ID': crypto.randomUUID() }, body: { candidate_token: candidateToken } })
export const revokeGuardedTab = (id: string, ownerToken: string) => apiRequest(`/api/v3/tab-choices/${encodeURIComponent(id)}/revoke`, { method: 'POST', headers: { Authorization: `Bearer ${ownerToken}`, 'X-Butler-Delivery-ID': crypto.randomUUID() }, body: {} })
