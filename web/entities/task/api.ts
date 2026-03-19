import { apiRequest } from '@shared/api/client'

export interface TaskItem {
  task_id: string
  run_id: string
  session_key: string
  status: string
  run_state: string
  needs_user_action: boolean
  waiting_reason: string
  source_channel: string
  user_action_channel: string
  updated_at: string
  finished_at?: string
}

export interface TaskListResponse {
  items: TaskItem[]
  total: number
  limit: number
  offset: number
}

export interface TaskSummaryBar {
  status: string
  risk_level: string
  source_channel: string
  started_at: string
  updated_at: string
  finished_at?: string | null
}

export interface TaskDetailRecord {
  task_id: string
  run_id: string
  session_key: string
  status: string
  run_state: string
  current_stage: string
  needs_user_action: boolean
  user_action_channel: string
  waiting_reason: string
  started_at: string
  updated_at: string
  finished_at?: string | null
  outcome_summary: string
  error_summary: string
  risk_level: string
  source_channel: string
  model_provider: string
  autonomy_mode: string
}

export interface TaskWaitingState {
  needs_user_action: boolean
  user_action_channel: string
  waiting_reason: string
  note: string
}

export interface TaskSource {
  channel: string
  session_key: string
  source_message_preview: string
  source_message_full: string
}

export interface TaskResult {
  outcome_summary: string
  has_result: boolean
}

export interface TaskError {
  error_type: string
  error_summary: string
  has_error: boolean
}

export interface TaskTransition {
  from_state: string
  to_state: string
  triggered_by: string
  transitioned_at: string
}

export interface TaskArtifact {
  artifact_id: string
  run_id: string
  session_key: string
  artifact_type: string
  title: string
  summary: string
  content_text: string
  content_json: string
  content_format: string
  source_type: string
  source_ref: string
  created_at: string
  updated_at: string
}

export interface TaskDetailResponse {
  task: TaskDetailRecord
  summary_bar: TaskSummaryBar
  source: TaskSource
  waiting_state: TaskWaitingState
  result: TaskResult
  error: TaskError
  timeline_preview: TaskTransition[]
  artifacts: TaskArtifact[]
}

export interface TaskActivityItem {
  activity_id: number
  run_id: string
  session_key: string
  activity_type: string
  title: string
  summary: string
  details_json: string
  actor_type: string
  severity: string
  created_at: string
}

export interface TaskActivityResponse {
  activity: TaskActivityItem[]
}

export interface DebugRun {
  run_id: string
  session_key: string
  status: string
  current_state: string
  model_provider: string
  provider_session_ref: string
  autonomy_mode: string
  metadata_json: string
  error_type: string
  error_message: string
  started_at: string
  updated_at: string
  finished_at?: string | null
}

export interface TranscriptMessage {
  message_id: string
  run_id: string
  role: string
  content: string
  tool_call_id?: string
  metadata_json?: string
  created_at: string
}

export interface TranscriptToolCall {
  tool_call_id: string
  tool_name: string
  args_json: string
  status: string
  runtime_target: string
  result_json: string
  error_json: string
  started_at: string
  finished_at?: string | null
}

export interface TaskDebugResponse {
  run: DebugRun
  transcript: {
    messages: TranscriptMessage[]
    tool_calls: TranscriptToolCall[]
  }
}

export interface TaskTranscriptResponse {
  run: DebugRun
  messages: TranscriptMessage[]
  tool_calls: TranscriptToolCall[]
}

export const fetchTasks = (query: Record<string, string | number | boolean | undefined> = {}) => {
  return apiRequest<TaskListResponse>('/api/v2/tasks', { query })
}

export const fetchTaskById = (taskId: string) => {
  return apiRequest<TaskDetailResponse>(`/api/v2/tasks/${taskId}`)
}

export const fetchTaskActivity = (taskId: string) => {
  return apiRequest<TaskActivityResponse>(`/api/v2/tasks/${taskId}/activity`)
}

export const fetchTaskArtifacts = (taskId: string) => {
  return apiRequest<{ artifacts: TaskArtifact[] }>(`/api/v2/tasks/${taskId}/artifacts`)
}

export const fetchTaskDebug = (taskId: string) => {
  return apiRequest<TaskDebugResponse>(`/api/v2/tasks/${taskId}/debug`)
}

export const fetchTaskTranscript = (taskId: string) => {
  return apiRequest<TaskTranscriptResponse>(`/api/v1/runs/${taskId}/transcript`)
}
