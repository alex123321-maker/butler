import { ownerAuthorityRequest, type ConverterPermission } from './owner-goals'

export interface ConverterSourceRef { source_id: string; revision: number; access_revision: number; content_hash: string }
export interface ConverterUse { run_id: string; task_id: string; task_revision: number; generation: number; source_ref: ConverterSourceRef; sha256: string }
export interface ConverterVersion {
  record: { converter_id: string; digest: string; version: string; generation: number; status: string; active: boolean; need_hash: string; profile_pin: string; permission: ConverterPermission }
  checks?: { passed: boolean; checker: string; checks: string[] }
  trials: number; passed_trials: number; uses: number; outputs: ConverterUse[]
}
export interface ConverterResponse { items?: ConverterVersion[]; error?: string }
export const converterRequest = (token: string, body?: Record<string, unknown>) => ownerAuthorityRequest<ConverterResponse>('/api/v4/converters', token, body)
