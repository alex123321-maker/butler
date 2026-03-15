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
    return await $fetch<T>(`${baseURL}${path}`, { method: 'POST', body })
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
