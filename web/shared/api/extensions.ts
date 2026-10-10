import { ownerAuthorityRequest } from './owner-goals'

export interface ExtensionManifest {
  id: string; version: string; description: string
  source: { uri: string; revision: string }
  runtime: { abi: string; profile: string }
  configuration: { name: string; type: string; required: boolean }[]
  credentials: { input: string; field: string; required: boolean }[]
  resources: { origin_setting: string; path_prefix: string }[]
  operations: { name: string; description: string; method: string; effect: string; receipt?: { protocol: string } }[]
  dependencies: { id: string; version: string; source: string; sha256: string }[]
}
export interface ExtensionState {
  extension_id: string; manifest_hash: string; generation: number
  status: 'staged' | 'disabled' | 'enabled' | 'quarantined'
  health: string; health_reason: string; healthy_until?: string
  reviewed: boolean; available: boolean
  configuration: Record<string, string>
  credential_refs: Record<string, { alias: string; field: string }>
  allowed_tools: string[]
}
export interface ExtensionItem { state: ExtensionState; manifest: ExtensionManifest }
export interface ExtensionResponse {
  items?: ExtensionItem[]; item?: ExtensionItem
  versions?: { manifest: ExtensionManifest; manifest_hash: string }[]
  staged_hash?: string; error?: string
  access?: Record<string, unknown>
}
export const extensionRequest = (token: string, request?: Record<string, unknown>) =>
  ownerAuthorityRequest<ExtensionResponse>('/api/v4/extensions', token, request)
