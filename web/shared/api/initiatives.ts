export interface InitiativeSource {
  source_id: string; owner_id?: string; goal_id: string; mandate_id: string; session_key: string
  project_ref: string; project_revision?: string; observer_kind?: string; digest_channel?: string
  check_manifest_hash?: string; check_tool_name?: string; check_resource?: string
  purpose: string; time_zone: string; task_goal: string; execution_policy: string
  completion_criteria: string[]; active_start_hour: number; active_end_hour: number
  failure_threshold: number; confidence_threshold: number; minimum_importance: number
  cooldown_seconds: number; enabled: boolean; revision: number
}
export interface InitiativeSchedule {
  schedule_id: string; source_id: string; time_zone: string; kind: string
  hour: number; minute: number; interval_seconds: number; missed_policy: string
  grace_seconds: number; next_at?: string; enabled: boolean; revision: number
}
export interface InitiativeView {
  sources: InitiativeSource[]; schedules: InitiativeSchedule[]
  initiatives: { initiative_id: string; source_id: string; status: string; reason: string; occurrences: number; confidence: number; importance: number; expected_model_calls: number; attention_cost: number; not_before: string; task_id?: string }[]
  events: { source_id: string; event_id: string; initiative_id?: string; verdict: string; observed_at: string }[]
  digests: { notification_id: string; channel: string; content: string; status: string; attempt_count: number; error_code?: string }[]
  attention_used_today: number; attention_daily_limit: number
}
