import { computed, type Ref } from 'vue'

// --- Types ---

export interface SessionRecord {
  session_key: string
  user_id: string
  channel: string
  created_at: string
  updated_at: string
}

export interface RunRecord {
  run_id: string
  session_key: string
  status: string
  current_state: string
  model_provider: string
  autonomy_mode: string
  started_at: string
  updated_at: string
  finished_at: string | null
  error_type?: string
  error_message?: string
}

export interface TranscriptMessage {
  message_id: string
  run_id: string
  role: string
  content: string
  tool_call_id?: string
  created_at: string
}

export interface TranscriptToolCall {
  tool_call_id: string
  run_id: string
  tool_name: string
  args_json: string
  status: string
  runtime_target: string
  started_at: string
  finished_at: string | null
  result_json: string
  error_json?: string
}

export interface MemoryLinkRecord {
	link_type: string
	target_type: string
	target_id: string
	metadata: string
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
	created_at: string
	updated_at: string
	links: MemoryLinkRecord[]
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
	created_at: string
	updated_at: string
	links: MemoryLinkRecord[]
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
	created_at: string
	updated_at: string
	links: MemoryLinkRecord[]
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

interface HealthResponse {
  status: string
  healthy: boolean
}

// --- API Client ---

export function useApiClient() {
  const config = useRuntimeConfig()
  const baseURL = config.public.apiBase as string

  async function get<T>(path: string): Promise<T> {
    return await $fetch<T>(`${baseURL}${path}`)
  }

  async function post<T>(path: string, body?: unknown): Promise<T> {
    return await $fetch<T>(`${baseURL}${path}`, {
      method: 'POST',
      body: body === undefined ? undefined : JSON.stringify(body),
      headers: { 'Content-Type': 'application/json' },
    })
  }

  return { get, post, baseURL }
}

// --- Health ---

export function useHealthCheck() {
  const { baseURL } = useApiClient()

  return useAsyncData<HealthResponse>('health', async () => {
    try {
      const resp = await $fetch<HealthResponse>(`${baseURL}/health`)
      return { status: resp.status || 'ok', healthy: true }
    } catch {
      return { status: 'unreachable', healthy: false }
    }
  }, {
    server: false,
    lazy: true,
  })
}

// --- Sessions ---

export function useSessions() {
  const { get } = useApiClient()

  return useAsyncData<SessionRecord[]>('sessions', async () => {
    const resp = await get<{ sessions: SessionRecord[] }>('/api/v1/sessions')
    return resp.sessions
  }, {
    server: false,
    default: () => [],
  })
}

// --- Session Detail ---

export function useSessionDetail(sessionKey: string) {
  const { get } = useApiClient()

  return useAsyncData<{ session: SessionRecord; runs: RunRecord[] }>(`session-${sessionKey}`, async () => {
    return await get<{ session: SessionRecord; runs: RunRecord[] }>(`/api/v1/sessions/${sessionKey}`)
  }, {
    server: false,
  })
}

// --- Run Detail ---

export function useRunDetail(runId: string) {
  const { get } = useApiClient()

  return useAsyncData<{ run: RunRecord }>(`run-${runId}`, async () => {
    return await get<{ run: RunRecord }>(`/api/v1/runs/${runId}`)
  }, {
    server: false,
  })
}

// --- Run Transcript ---

export function useRunTranscript(runId: string) {
  const { get } = useApiClient()

  return useAsyncData<{ run: RunRecord; messages: TranscriptMessage[]; tool_calls: TranscriptToolCall[] }>(`transcript-${runId}`, async () => {
    return await get<{ run: RunRecord; messages: TranscriptMessage[]; tool_calls: TranscriptToolCall[] }>(`/api/v1/runs/${runId}/transcript`)
  }, {
    server: false,
  })
}

// --- Doctor Types ---

export interface DoctorCheckResult {
  name: string
  status: string
  message?: string
  duration: string
  checked_at: string
}

export interface DoctorReportData {
  status: string
  checked_at: string
  checks: DoctorCheckResult[]
  config?: Array<{ key: string; effective_value: string; source: string; validation_status: string }>
	input_id?: string
	execution?: { status: string; run_id?: string; task_id?: string }
	check_error?: string
	memory?: {
		status: string; checked_at: string
		components: Record<string, { configured: boolean; effective: boolean; status: string; reason?: string; checked_at: string }>
		legacy_queue: { status: string; depth?: number; reason?: string }
		jobs: { status: string; observation?: { pending: number; running: number; completed: number; failed: number; retries: number; oldest_pending_seconds: number | null; reasons: Record<string, number> } }
		indexes: { status: string; truncated?: boolean; generations?: Array<{ generation_id: string; scope_type: string; scope_id: string; space_id: string; state: string; active: boolean; verified_at: string | null; generation_proof?: unknown }> }
		answer_quality: { status: string; reason: string; configuration: string }
	}
}

export interface DoctorReport {
  id: number
  status: string
  checked_at: string
  report: DoctorReportData
}

export function useMemoryScope(scopeType: Ref<string> | string, scopeID: Ref<string> | string) {
	const { get } = useApiClient()
	const resolvedScopeType = typeof scopeType === 'string' ? computed(() => scopeType) : scopeType
	const resolvedScopeID = typeof scopeID === 'string' ? computed(() => scopeID) : scopeID

	return useAsyncData<MemoryScopeView>(
		() => `memory-${resolvedScopeType.value}-${resolvedScopeID.value}`,
		async () => {
			if (!resolvedScopeType.value || !resolvedScopeID.value) {
				return { scope_type: resolvedScopeType.value || '', scope_id: resolvedScopeID.value || '' }
			}
			return await get<MemoryScopeView>(`/api/v1/memory?scope_type=${encodeURIComponent(resolvedScopeType.value)}&scope_id=${encodeURIComponent(resolvedScopeID.value)}`)
		},
		{
			server: false,
			default: () => ({ scope_type: typeof scopeType === 'string' ? scopeType : '', scope_id: typeof scopeID === 'string' ? scopeID : '' }),
			watch: [resolvedScopeType, resolvedScopeID],
		}
	)
}
