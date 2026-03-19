import { ref, onUnmounted, type Ref } from 'vue'

// --- Types ---

export interface RunEvent {
  event_id: string
  run_id: string
  session_key: string
  event_type: string
  timestamp: string
  payload: Record<string, unknown>
}

export interface UseRunEventsReturn {
  /** All received events in order */
  events: Ref<RunEvent[]>
  /** Whether the SSE connection is active */
  connected: Ref<boolean>
  /** Whether the stream ended (run completed/failed or stream_end received) */
  ended: Ref<boolean>
  /** Last error, if any */
  error: Ref<string | null>
  /** Manually close the connection */
  close: () => void
}

const EVENT_TYPES = [
  'state_transition',
  'memory_loaded',
  'tools_loaded',
  'prompt_assembled',
  'assistant_delta',
  'assistant_final',
  'tool_started',
  'tool_completed',
  'approval_requested',
  'approval_resolved',
  'run_error',
  'run_completed',
  'stream_end',
] as const

/** Maximum number of reconnection attempts before giving up. */
const MAX_RECONNECT_ATTEMPTS = 8

/** Initial reconnection delay in milliseconds. */
const INITIAL_RECONNECT_DELAY_MS = 1000

/** Maximum reconnection delay in milliseconds. */
const MAX_RECONNECT_DELAY_MS = 30000

/**
 * useRunEvents connects to the SSE endpoint for a run and provides a reactive
 * list of observability events. It handles catch-up replay, live streaming,
 * automatic reconnection with exponential backoff, and clean teardown.
 */
export function useRunEvents(runId: string): UseRunEventsReturn {
  const config = useRuntimeConfig()
  const baseURL = config.public.apiBase as string

  const events = ref<RunEvent[]>([])
  const connected = ref(false)
  const ended = ref(false)
  const error = ref<string | null>(null)

  let eventSource: EventSource | null = null
  let reconnectAttempts = 0
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let closed = false // true once close() is called manually or stream_end is received

  /** Set of event_ids already received, to deduplicate on reconnect (catch-up replay). */
  const seenEventIds = new Set<string>()

  function connect() {
    if (closed || ended.value) return

    const url = `${baseURL}/api/v1/runs/${encodeURIComponent(runId)}/events`
    eventSource = new EventSource(url)

    eventSource.onopen = () => {
      connected.value = true
      error.value = null
      reconnectAttempts = 0 // reset backoff on successful connection
    }

    eventSource.onerror = () => {
      connected.value = false
      if (eventSource) {
        eventSource.close()
        eventSource = null
      }
      if (!closed && !ended.value) {
        error.value = 'SSE connection lost'
        scheduleReconnect()
      }
    }

    for (const type of EVENT_TYPES) {
      eventSource.addEventListener(type, (e: MessageEvent) => {
        try {
          const evt: RunEvent = JSON.parse(e.data)

          if (type === 'stream_end') {
            ended.value = true
            teardown()
            return
          }

          // Deduplicate events (replay events on reconnect may repeat).
          if (evt.event_id && seenEventIds.has(evt.event_id)) {
            return
          }
          if (evt.event_id) {
            seenEventIds.add(evt.event_id)
          }

          events.value = [...events.value, evt]
        } catch {
          // Malformed event — skip.
        }
      })
    }
  }

  function scheduleReconnect() {
    if (closed || ended.value) return
    if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
      error.value = 'SSE reconnection failed after maximum attempts'
      return
    }

    const delay = Math.min(
      INITIAL_RECONNECT_DELAY_MS * Math.pow(2, reconnectAttempts),
      MAX_RECONNECT_DELAY_MS,
    )
    reconnectAttempts++

    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      connect()
    }, delay)
  }

  function teardown() {
    closed = true
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
    connected.value = false
  }

  function close() {
    teardown()
  }

  // Auto-connect on composable creation.
  connect()

  // Clean up on component unmount.
  onUnmounted(() => {
    close()
  })

  return {
    events,
    connected,
    ended,
    error,
    close,
  }
}
