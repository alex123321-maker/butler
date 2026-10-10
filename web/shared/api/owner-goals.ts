import { apiRequest } from './client'

export interface OwnerGoal {
  goal_id: string; owner_id?: string; revision: number
  kind: 'objective' | 'preference' | 'constraint' | 'assumption'
  status: 'active' | 'paused' | 'revoked' | 'proposed'
  title: string; purpose: string; constraints: string[]; expires_at: string
  source?: { author: string; channel: string; delivery_id: string; parent_ref?: string }
}
export interface OwnerMandate {
  mandate_id: string; revision: number; goal_id: string; goal_revision: number
  scope_kind: string; status: 'active' | 'paused' | 'revoked'; purpose: string
  tools: string[]; resource_scopes: { origin: string; path_prefix: string }[]
  effects: string[]; credential_refs: { alias: string; field: string }[]
  privacy: string; expires_at: string; budget_deadline?: string
  budget: Record<string, number>; allow_initiative: boolean; intervention: string[]; reporting: string
  active_hours?: { timezone: string; start_minute: number; end_minute: number; weekdays: number[] }
  deny_tools?: string[]; deny_resources?: { origin: string; path_prefix: string }[]
  extensions?: { source: string; extension_id: string; manifest_hash: string; tools: string[] }[]
  source?: { author: string; channel: string; delivery_id: string }
}
export interface OwnerAuthorityView {
  goals: OwnerGoal[]; mandates: OwnerMandate[]; pending_propagation: number
  obligations: { task_id: string; goal_id: string; goal_revision: number; mandate_id: string; mandate_revision: number; initiative_reason: string; observation_ref: string; task_status: string; authority_current: boolean }[]
}
export interface AuthorityReceipt { event_id: string; subject_id: string; revision: number; propagation: string }

export const ownerAuthorityRequest = <T>(path: string, token: string, body?: unknown) => {
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` }
  if (body !== undefined) headers['X-Butler-Delivery-ID'] = [...crypto.getRandomValues(new Uint8Array(16))].map(value => value.toString(16).padStart(2, '0')).join('')
  return apiRequest<T>(path, { method: body === undefined ? 'GET' : 'POST', headers, body })
}
