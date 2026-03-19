import { apiRequest } from '@shared/api/client'

export interface MemoryLinkRecord {
  link_type: string
  target_type: string
  target_id: string
  metadata: string
}

// Confirmation states for memory entries
export type ConfirmationState = 'pending' | 'confirmed' | 'rejected' | 'auto_confirmed'

// Effective status for memory entries
export type EffectiveStatus = 'active' | 'inactive' | 'suppressed' | 'expired' | 'deleted'

// Memory type identifier
export type MemoryType = 'profile' | 'episodic' | 'chunk' | 'working'

// Capabilities for a memory type
export interface MemoryCapabilities {
  editable: boolean
  confirmable: boolean
  suppressible: boolean
  deletable: boolean
  hard_deletable: boolean
}

export interface WorkingMemoryRecord {
  memory_type: string
  session_key: string
  run_id: string
  goal: string
  entities_json: string
  pending_steps_json: string
  scratch_json: string
  status: string
  source_type: string
  source_id: string
  provenance: string
  created_at: string
  updated_at: string
}

export interface ProfileMemoryRecord {
  id: number
  memory_type: string
  scope_type: string
  scope_id: string
  key: string
  value_json: string
  summary: string
  source_type: string
  source_id: string
  provenance: string
  confidence: number
  status: string
  confirmation_state: ConfirmationState
  effective_status: EffectiveStatus
  suppressed: boolean
  expires_at?: string
  edited_by?: string
  edited_at?: string
  created_at: string
  updated_at: string
  links: MemoryLinkRecord[]
  capabilities: MemoryCapabilities
}

export interface EpisodicMemoryRecord {
  id: number
  memory_type: string
  scope_type: string
  scope_id: string
  summary: string
  content: string
  source_type: string
  source_id: string
  provenance: string
  confidence: number
  status: string
  tags_json: string
  confirmation_state: ConfirmationState
  effective_status: EffectiveStatus
  suppressed: boolean
  expires_at?: string
  edited_by?: string
  edited_at?: string
  created_at: string
  updated_at: string
  links: MemoryLinkRecord[]
  capabilities: MemoryCapabilities
}

export interface ChunkMemoryRecord {
  id: number
  memory_type: string
  scope_type: string
  scope_id: string
  title: string
  summary: string
  content: string
  source_type: string
  source_id: string
  provenance: string
  confidence: number
  status: string
  tags_json: string
  effective_status: EffectiveStatus
  suppressed: boolean
  expires_at?: string
  edited_by?: string
  edited_at?: string
  created_at: string
  updated_at: string
  links: MemoryLinkRecord[]
  capabilities: MemoryCapabilities
}

export interface MemoryScopeView {
  scope_type: string
  scope_id: string
  limit?: number
  working?: WorkingMemoryRecord
  profile?: ProfileMemoryRecord[]
  episodic?: EpisodicMemoryRecord[]
  chunks?: ChunkMemoryRecord[]
}

// Request types
export interface MemoryPatchRequest {
  value_json?: string
  summary?: string
}

// Fetch memory by scope
export const fetchMemory = (query: Record<string, string | number | undefined> = {}) => {
  return apiRequest<MemoryScopeView>('/api/v1/memory', { query })
}

// Get a single memory item by type and ID
export const fetchMemoryItem = (memoryType: MemoryType, id: number) => {
  return apiRequest<ProfileMemoryRecord | EpisodicMemoryRecord | ChunkMemoryRecord>(
    '/api/v2/memory/item',
    { query: { memory_type: memoryType, id } }
  )
}

// Update a memory entry (only for editable types like profile)
export const patchMemory = (memoryType: MemoryType, id: number, data: MemoryPatchRequest) => {
  return apiRequest<ProfileMemoryRecord>(
    '/api/v2/memory/patch',
    { method: 'PATCH', query: { memory_type: memoryType, id }, body: data }
  )
}

// Delete a memory entry (soft or hard depending on type)
export const deleteMemory = (memoryType: MemoryType, id: number) => {
  return apiRequest<ProfileMemoryRecord | EpisodicMemoryRecord | { status: string }>(
    '/api/v2/memory/delete',
    { method: 'DELETE', query: { memory_type: memoryType, id } }
  )
}

// Confirm a pending memory entry
export const confirmMemory = (memoryType: MemoryType, id: number) => {
  return apiRequest<ProfileMemoryRecord | EpisodicMemoryRecord>(
    '/api/v2/memory/confirm',
    { method: 'POST', query: { memory_type: memoryType, id } }
  )
}

// Reject a pending memory entry
export const rejectMemory = (memoryType: MemoryType, id: number) => {
  return apiRequest<ProfileMemoryRecord | EpisodicMemoryRecord>(
    '/api/v2/memory/reject',
    { method: 'POST', query: { memory_type: memoryType, id } }
  )
}

// Suppress a memory entry (hidden from retrieval but kept for audit)
export const suppressMemory = (memoryType: MemoryType, id: number) => {
  return apiRequest<ProfileMemoryRecord | EpisodicMemoryRecord | ChunkMemoryRecord>(
    '/api/v2/memory/suppress',
    { method: 'POST', query: { memory_type: memoryType, id } }
  )
}

// Unsuppress a memory entry
export const unsuppressMemory = (memoryType: MemoryType, id: number) => {
  return apiRequest<ProfileMemoryRecord | EpisodicMemoryRecord | ChunkMemoryRecord>(
    '/api/v2/memory/unsuppress',
    { method: 'POST', query: { memory_type: memoryType, id } }
  )
}
