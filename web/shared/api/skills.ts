import { ownerAuthorityRequest } from './owner-goals'

export interface SkillManifest {
  id: string; version: string; description: string
  source: { uri: string; revision: string }
  when: string[]; inputs: string[]; not_applicable: string[]
  dependencies: { tool: string; contract_hash: string; contract_revision: string }[]
  steps: { id: string; tool: string; intent: string; success_check: string; on_error: string; retry: string; approval_checkpoint: boolean }[]
}
export interface SkillRecord {
  skill_id: string; digest: string; version: string; generation: number
  status: 'candidate' | 'tested' | 'active' | 'obsolete' | 'revoked'
  active: boolean; body_available: boolean
  body_source: { source_id: string; revision: number; content_hash: string }
  package: { manifest: SkillManifest; digest: string }
}
export interface SkillResponse {
  items?: SkillRecord[]; item?: SkillRecord; error?: string
  validation?: { passed: boolean; checks: string[] }
  usages?: { run_id: string; task_id: string; task_revision: number; loaded_at: string }[]
  quality?: { verified_successes: number; unverified_attempts: number; known_tool_failures: number; baseline: unknown[]; outcomes: unknown[] }
}
export const skillRequest = (token: string, request?: Record<string, unknown>) =>
  ownerAuthorityRequest<SkillResponse>('/api/v4/skills', token, request)
