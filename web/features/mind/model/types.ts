export interface Task {
  task_id: string
  run_id: string
  session_key: string
  status: string
  task_status?: string
  task_revision?: number
  goal?: string
  run_state: string
  needs_user_action: boolean
  waiting_reason: string
  source_channel: string
  updated_at: string
  started_at?: string
  result_verified?: boolean
  legacy?: boolean
}
export interface Source {
	match?: 'text' | 'semantic' | 'both'
  source_id: string
  source_kind: string
  title: string
  body: string
  status: string
  revision: number
  access_revision?: number
  content_hash?: string
  extracted_at: string
  created_at?: string
  scope_type: string
  scope_id: string
  author?: string
  event_time?: string | null
  origin?: { sensitivity: string; kind?: string; source_refs?: string[]; parent_refs?: string[] }
  redacted?: boolean
}
export interface MemoryPage {
  items: Source[]
  has_more: boolean
  mode: 'recent' | 'text' | 'semantic' | 'hybrid'
  next_offset?: number
  diagnostics: string[]
}
export interface Inspection {
  source: Source
  fragments: { chunk_id: number; title: string; text: string }[]
  revisions?: { revision: number; title?: string; body: string; author?: string; event_time?: string; created_at?: string; origin?: Source['origin'] }[]
  revision_limit?: number
}
export interface MemoryClaim {
  claim_id: string
  key: string
  scope_type: string
  scope_id: string
  summary: string
  status: string
  source_id: string
  source_revision: number
}
export interface Approval {
  approval_id: string
  run_id: string
  status: string
  tool_name?: string
  summary?: string
  expires_at?: string
  owner_facts?: {
    summary: string
    effect: string
    resource: string
    task_id?: string
    task_revision: number
    constraints_revision: number
    goal_id?: string
    goal_revision?: number
    mandate_id?: string
    mandate_revision?: number
    credential_reference_count?: number
    contract_hash: string
    args_hash: string
  }
}
export interface Activity {
  activity_id: number
  run_id?: string
  details_json?: string
  title: string
  summary: string
  activity_type: string
  created_at: string
}
export interface Message {
  message_id: string
  role: string
  content: string
  created_at: string
  kind?: string
  visibility?: string
  published?: boolean
  metadata_json?: string
}
export interface Receipt {
  input_id: string
  task_id?: string
  run_id?: string
  session_key: string
  status: string
  checkpoint?: string
  error_code?: string
  error_message?: string
}
export interface TaskDetail {
  task: {
    task_id: string
    status: string
    run_state: string
    current_stage: string
    outcome_summary: string
    error_summary: string
    started_at: string
    finished_at?: string
  }
  durable_task?: {
    goal: string
    constraints: string[]
    completion_criteria?: string[]
  }
  result?: { has_result: boolean; verified?: boolean; outcome_summary: string }
  final_result?: {
    result_id?: string
    state?: string
    answer_state?: string
    effects?:
      | {
          action_id: string
          effect_state: string
          evidence_refs?: string[] | null
        }[]
      | null
    limitations?: string[] | null
    privacy_redacted?: boolean
  } | null
  delivery?: { channel?: string; state?: string }
  artifacts?: { artifact_id: string; title: string; summary: string }[]
  run_ids?: string[]
}
export interface DurableTask {
  task_id: string
  revision: number
  status: string
  goal: string
  working?: { plan?: string[]; pending_actions?: string[] }
  execution_policy?: string
}
export interface CalendarItem {
  id: string
  title: string
  at: string
  status: string
  done: boolean
  taskId?: string
  source: 'task' | 'schedule'
  description: string
}
export interface Schedule {
  schedule_id: string
  source_id: string
  time_zone: string
  next_at?: string
  enabled: boolean
  revision: number
  kind: string
  hour: number
  minute: number
  interval_seconds: number
  missed_policy: string
  grace_seconds: number
}
export interface InitiativeData {
  sources: {
    source_id: string
    purpose: string
    task_goal: string
    enabled: boolean
  }[]
  schedules: Schedule[]
  digests: {
    notification_id: string
    content: string
    channel: string
    status: string
    error_code?: string
  }[]
}
export interface Notice {
  id: string
  title: string
  body: string
  source: string
  at?: string
  tone: 'sage' | 'sand' | 'clay'
  taskId?: string
  approvalId?: string
  deliveryState?: string
}
export type Panel =
  | ''
  | 'settings'
  | 'tasks'
  | 'task'
  | 'journal'
  | 'memory'
  | 'note'
  | 'approvals'
  | 'notice'
  | 'tools'
  | 'advanced'
  | 'history'
  | 'input'
