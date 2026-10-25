import { computed, ref, watch, type Ref } from 'vue'

export interface PublicationFile {
  artifact_ref: string
  name: string
  mime: string
  sha256: string
  size: number
}
export interface PublicationBlock {
  type: string
  text?: string
  items?: string[]
  rows?: string[][]
  artifact_ref?: string
  alt?: string
}
export interface WidgetAnswer { option_ids?: string[]; text?: string }
export interface WidgetView {
  interaction_ref: string
  version: number
  state: 'open' | 'resolved' | 'expired' | 'cancelled' | 'superseded'
  draft_version: number
  draft: WidgetAnswer
  answer: WidgetAnswer
  expires_at?: string
}
export interface WidgetInput {
  type: 'single_choice' | 'multi_choice' | 'text'
  options?: { id: string; label: string; description?: string; image_ref?: string }[]
  allow_text_answer?: boolean
  min_selections?: number
  max_selections?: number
  min_length?: number
  max_length?: number
  placeholder?: string
}
export interface Publication {
  publication_ref: string
  sequence: number
  session_ref?: string
  task_ref?: string
  task_revision?: number
  kind?: string
  created_at: string
  available: boolean
  interaction?: WidgetView
  payload?: {
    purpose?: string
    text?: string
    title?: string
    summary?: string
    images?: { artifact_ref: string; alt: string; caption?: string }[]
    blocks?: PublicationBlock[]
    attachments?: string[]
    question?: string
    input?: WidgetInput
  }
  files: PublicationFile[]
  parts: {
    channel: string
    binding_id: string
    binding_version: number
    index: number
    state: string
    ack?: string
    error?: string
    manifest?: unknown
    projection_state?: string
  }[]
}
interface Profile { ready: boolean; binding_id: string; binding_version: number; drain_only?: boolean }
export interface PublicationInput { message_id: string; session_ref: string; content: string; created_at: string }
interface Page { items: Publication[]; messages?: PublicationInput[]; cursor: number; has_more: boolean }
export interface ExecutionNotice {
  notice_ref: string
  input_ref: string
  run_ref?: string
  code: string
  reason?: string
  text: string
  approval_ref?: string
  delivery_state: string
  created_at: string
}

// The stream carries invalidation only. Every reconnect replaces the loaded
// pages from durable REST, including redacted entries and delivery outcomes.
export function useCommunicationFeed(
  epoch: Ref<number>,
  read: (path: string, signal?: AbortSignal) => Promise<Response>,
) {
  const communicationProfile = ref<Profile | null>(null)
  const publications = ref<Publication[]>([])
  const publicationInputs = ref<PublicationInput[]>([])
  const executionNotices = ref<ExecutionNotice[]>([])
  const executionNoticeError = ref('')
  const publicationMore = ref(false)
  const communicationError = ref('')
  const replyPublication = ref<Publication | null>(null)
  const communicationBlocked = ref(false)
  const communicationDraining = computed(() => communicationProfile.value?.drain_only === true)
  let pages = 1, loading = false, refreshAgain = false, cursor = 0
  let stream: AbortController | undefined
  let timer: ReturnType<typeof setTimeout> | undefined
  function clearCommunications() {
    stream?.abort(); clearTimeout(timer)
    communicationProfile.value = null
    publications.value = []
    publicationInputs.value = []
    executionNotices.value = []
    executionNoticeError.value = ''
    replyPublication.value = null
    communicationError.value = ''
    communicationBlocked.value = false
    publicationMore.value = false
    pages = 1; loading = false; refreshAgain = false; cursor = 0
  }
  watch(epoch, clearCommunications, { flush: 'sync' })
  async function loadPublications(more = false) {
    if (!communicationProfile.value) return
    // Retain an explicit history expansion even while a background read owns
    // the request. The queued refresh must use the enlarged visible window.
    if (more) pages++
    if (loading) { refreshAgain = true; return }
    const stamp = epoch.value
    loading = true
    try {
      const items: Publication[] = []
      const messages: PublicationInput[] = []
      let states: ExecutionNotice[] = []
      let stateError = ''
      try {
        const statesResponse = await read('/api/v3/communications/states')
        if (statesResponse.ok) {
          const data = await statesResponse.json() as { ready?: boolean; items?: ExecutionNotice[] }
          if (data.ready === true && Array.isArray(data.items)) states = data.items
        } else if (statesResponse.status !== 404) throw new Error('states_unavailable')
      } catch {
        stateError = 'Состояние выполнения временно недоступно.'
      }
      if (stamp !== epoch.value) return
      let after = 0, hasMore = false
      const targetItems = pages * 50
      // Server cursor pages are opaque and can contain one fully checked
      // publication. Preserve the previous visible history batch independently
      // of that transaction size, without retaining locks across HTTP calls.
      for (let page = 0; page < targetItems; page++) {
        const response = await read(`/api/v3/communications?cursor=${after}`)
        if (!response.ok) throw new Error('unavailable')
        const data = await response.json() as Page
        if (stamp !== epoch.value) return
        if (!Number.isSafeInteger(data.cursor) || data.cursor < after
          || (data.cursor === after && (data.has_more || data.items.length > 0))
          || (data.has_more && data.items.length === 0)) throw new Error('invalid_cursor')
        items.push(...data.items); after = data.cursor; hasMore = data.has_more
        messages.push(...data.messages || [])
        if (!hasMore || items.length >= targetItems) break
      }
      publications.value = [...new Map(items.map(p => [p.publication_ref, p])).values()]
      publicationInputs.value = [...new Map(messages.map(m => [m.message_id, m])).values()]
      executionNotices.value = states
      executionNoticeError.value = stateError
      publicationMore.value = hasMore
      if (replyPublication.value) {
        const latest = publications.value.find(p => p.publication_ref === replyPublication.value?.publication_ref)
        replyPublication.value = latest?.available ? latest : null
      }
      communicationError.value = ''
      communicationBlocked.value = false
    } catch {
      if (stamp === epoch.value) {
        // Old private content cannot stay visible after an uncertain recheck.
        publications.value = []; publicationInputs.value = []; replyPublication.value = null
        executionNotices.value = []
        communicationError.value = 'Публикации сейчас недоступны. Доступ проверяется заново.'
        communicationBlocked.value = true
      }
    } finally {
      if (stamp === epoch.value) {
        loading = false
        if (refreshAgain) { refreshAgain = false; void loadPublications() }
      }
    }
  }
  async function openStream() {
    const stamp = epoch.value
    if (!communicationProfile.value) return
    stream?.abort(); stream = new AbortController()
    const owned = stream
    try {
      await loadPublications()
      const response = await read(`/api/v3/communications/events?cursor=${cursor}`, owned.signal)
      if (!response.ok || !response.body) throw new Error('stream_unavailable')
      const reader = response.body.getReader(), decoder = new TextDecoder()
      let buffer = ''
      try {
        while (!owned.signal.aborted && stamp === epoch.value) {
          const next = await reader.read()
          if (next.done) break
          buffer += decoder.decode(next.value, { stream: true }).replace(/\r/g, '')
          if (buffer.length > 65536) throw new Error('invalid_stream')
          let end: number
          while ((end = buffer.indexOf('\n\n')) !== -1) {
            const frame = buffer.slice(0, end); buffer = buffer.slice(end + 2)
            if (frame.includes('event: reset')) {
              publications.value = []; publicationInputs.value = []; replyPublication.value = null
              executionNotices.value = []
              communicationBlocked.value = true
              throw new Error('stream_reset')
            }
            if (frame.includes('event: refresh')) {
              const id = /^id: (\d+)$/m.exec(frame)?.[1]
              if (id && Number.isSafeInteger(Number(id))) cursor = Number(id)
              void loadPublications()
            }
          }
        }
      } finally { await reader.cancel().catch(() => {}); reader.releaseLock() }
    } catch {
      if (stamp === epoch.value && !owned.signal.aborted) await loadPublications()
    } finally {
      if (stamp === epoch.value && !owned.signal.aborted)
        timer = setTimeout(() => void openStream(), 1500)
    }
  }
  async function connectCommunications() {
    const stamp = epoch.value
    try {
      const response = await read('/api/v3/communications/profile')
      if (stamp !== epoch.value) return
      if (response.status === 404) return // Ordinary legacy startup is unchanged.
      if (!response.ok) throw new Error('profile_unavailable')
      const profile = await response.json() as Profile
      if (stamp !== epoch.value) return
      if (!profile.ready || !profile.binding_id || !Number.isSafeInteger(profile.binding_version) || profile.binding_version < 1)
        throw new Error('profile_unavailable')
      communicationProfile.value = profile
      void openStream()
    } catch {
      if (stamp === epoch.value) {
        communicationBlocked.value = true
        communicationError.value = 'Режим общения пока не удалось проверить. Обновите подключение.'
      }
    }
  }
  return { communicationProfile, publications, publicationInputs, executionNotices, executionNoticeError, publicationMore, communicationError,
    communicationBlocked, communicationDraining, replyPublication, connectCommunications, loadPublications, clearCommunications }
}
