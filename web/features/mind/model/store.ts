import { defineStore } from 'pinia'
import { computed, onScopeDispose, ref, watch } from 'vue'
import type { TranscriptToolCall } from '~/composables/useApi'
import {
  calendarTasks,
  isWorking,
  newID,
  publicMessage,
  taskStatus,
  taskTitle,
} from './presentation'
import type {
  Activity,
  Approval,
  DurableTask,
  InitiativeData,
  Inspection,
  MemoryPage,
  Message,
  Notice,
  Panel,
  Receipt,
  Source,
  Task,
  TaskDetail,
} from './types'

interface PendingRequest {
  path: string
  body: unknown
  delivery: string
  epoch: number
}
interface PendingNote {
  title: string
  body: string
  delivery: string
  epoch: number
}
export const useMind = defineStore('mind', () => {
  const config = useRuntimeConfig()
  const base = String(config.public.apiBase || '').replace(/\/$/, '')
  const token = ref(''),
    connected = ref(false),
    connecting = ref(false),
    connectionError = ref('')
  const epoch = ref(0),
    tasks = ref<Task[]>([]),
    approvals = ref<Approval[]>([]),
    initiatives = ref<InitiativeData | null>(null)
  const selectedId = ref(''),
    detail = ref<TaskDetail | null>(null),
    durable = ref<DurableTask | null>(null),
    activity = ref<Activity[]>([]),
    messages = ref<Message[]>([]),
    toolCalls = ref<TranscriptToolCall[]>([])
  const memories = ref<Source[]>([]),
    memoryMode = ref('recent'),
    memoryDiagnostics = ref<string[]>([]),
    memoryHasMore = ref(false),
    memoryQuery = ref(''),
    memoryBusy = ref(false),
    memoryError = ref(''),
    inspection = ref<Inspection | null>(null)
  const panel = ref<Panel>(''),
    advanced = ref(''),
    selectedNotice = ref<Notice | null>(null),
    error = ref(''),
    busy = ref(false),
    loadingTask = ref(false)
  const draft = ref(''),
    receipt = ref<Receipt | null>(null),
    sending = ref(false),
    pendingIngress = ref<PendingRequest | null>(null),
    pendingControl = ref<PendingRequest | null>(null)
  const sessionKey = ref(''),
    motion = ref(true),
    quiet = ref(false),
    noticeHidden = ref<string[]>([])
  const pendingNote = ref<PendingNote | null>(null)
  let disposed = false,
    controller = new AbortController(),
    selection = 0,
    memorySelection = 0,
    memorySearch = 0
  let poll: ReturnType<typeof setTimeout> | undefined,
    receiptPoll: ReturnType<typeof setTimeout> | undefined
  const selectedTask = computed(
    () => tasks.value.find((task) => task.task_id === selectedId.value) || null,
  )
  const runId = computed(
    () =>
      selectedTask.value?.run_id ||
      (receipt.value?.task_id === selectedId.value
        ? receipt.value?.run_id
        : '') ||
      '',
  )
  const working = computed(() => isWorking(selectedTask.value))
  const conversation = computed(() => messages.value.filter(publicMessage))
  const calendar = computed(() => {
    const scheduled = (initiatives.value?.schedules || [])
      .filter((s) => s.enabled && s.next_at)
      .map((s) => ({
        id: s.schedule_id,
        title:
          initiatives.value?.sources?.find(
            (source) => source.source_id === s.source_id,
          )?.purpose || 'Запланированное поручение',
        at: s.next_at!,
        status: 'scheduled',
        done: false,
        source: 'schedule' as const,
        description: 'Запланировано',
      }))
    return [...calendarTasks(tasks.value), ...scheduled]
  })
  const notices = computed<Notice[]>(() => {
    const result: Notice[] = approvals.value
      .filter((a) => a.status === 'pending')
      .map((a) => ({
        id: a.approval_id,
        title: a.owner_facts?.summary || a.summary || 'Нужно разрешение',
        body: a.owner_facts?.resource || a.tool_name || '',
        source: 'Разрешение',
        at: a.expires_at,
        tone: 'sand',
        approvalId: a.approval_id,
        taskId: a.owner_facts?.task_id,
      }))
    for (const task of tasks.value.filter(
      (t) => t.needs_user_action && !result.some((n) => n.taskId === t.task_id),
    ))
      result.push({
        id: task.task_id,
        title: taskTitle(task),
        body: task.waiting_reason || 'Нужно ваше участие',
        source: 'Поручение',
        at: task.updated_at,
        tone: taskStatus(task) === 'blocked' ? 'clay' : 'sand',
        taskId: task.task_id,
      })
    for (const d of initiatives.value?.digests || [])
      result.push({
        id: d.notification_id,
        title: d.status === 'sent' ? 'Сводка отправлена' : 'Сводка сохранена',
        body: d.content,
        source: d.channel,
        tone: d.error_code ? 'clay' : 'sage',
        deliveryState: d.status,
      })
    return result.filter((n) => !noticeHidden.value.includes(n.id))
  })
  const current = (stamp: number) =>
    !disposed && stamp === epoch.value && !!token.value.trim()
  function resetPrivate() {
    epoch.value++
    controller.abort()
    controller = new AbortController()
    clearTimeout(poll)
    clearTimeout(receiptPoll)
    connected.value = false
    connecting.value = false
    tasks.value = []
    approvals.value = []
    initiatives.value = null
    selection++
    memorySelection++
    memorySearch++
    selectedId.value = ''
    detail.value = null
    durable.value = null
    activity.value = []
    messages.value = []
    toolCalls.value = []
    memories.value = []
    inspection.value = null
    memoryQuery.value = ''
    memoryDiagnostics.value = []
    memoryError.value = ''
    memoryBusy.value = false
    pendingIngress.value = null
    pendingControl.value = null
    receipt.value = null
    draft.value = ''
    sessionKey.value = ''
    sending.value = false
    pendingNote.value = null
    error.value = ''
    busy.value = false
    loadingTask.value = false
    connectionError.value = ''
    noticeHidden.value = []
    selectedNotice.value = null
    if (panel.value !== 'settings') panel.value = ''
  }
  watch(token, resetPrivate, { flush: 'sync' })
  async function request<T>(
    path: string,
    body?: unknown,
    options: { method?: string; delivery?: string } = {},
  ): Promise<T> {
    const stamp = epoch.value,
      proof = token.value.trim()
    if (!current(stamp)) throw new Error('owner_changed')
    const headers: Record<string, string> = {
      Authorization: `Bearer ${proof}`,
      'Content-Type': 'application/json',
    }
    if (body !== undefined)
      headers['X-Butler-Delivery-ID'] = options.delivery || newID()
    const response = await fetch(`${base}${path}`, {
      method: options.method || (body === undefined ? 'GET' : 'POST'),
      body: body === undefined ? undefined : JSON.stringify(body),
      headers,
      signal: controller.signal,
      cache: 'no-store',
    })
    if (!current(stamp)) throw new Error('owner_changed')
    if (!response.ok)
      throw new Error(
        response.status === 409
          ? 'version_changed'
          : response.status === 401 || response.status === 403
            ? 'owner_denied'
            : 'unavailable',
      )
    const result = (await response.json()) as T
    if (!current(stamp)) throw new Error('owner_changed')
    return result
  }
  async function connect() {
    if (!token.value.trim() || connecting.value) return
    const stamp = epoch.value
    connecting.value = true
    connectionError.value = ''
    try {
      const access = await request<{ authenticated: boolean }>(
        '/api/v4/memory/access',
      )
      if (access.authenticated !== true) throw new Error('owner_denied')
      if (!current(stamp)) return
      connected.value = true
      await Promise.allSettled([loadMemory(''), refresh()])
      schedulePoll()
    } catch {
      if (current(stamp))
        connectionError.value =
          'Не удалось подключиться. Проверьте доступ владельца и соединение с Butler.'
    } finally {
      if (current(stamp)) connecting.value = false
    }
  }
  async function refresh() {
    const stamp = epoch.value
    if (!connected.value) return
    const result = await Promise.allSettled([
      request<{ tasks?: Task[]; items?: Task[] }>(
        '/api/v2/tasks?limit=100&sort=updated_at_desc',
      ),
      request<{ approvals: Approval[] }>('/api/v2/approvals'),
      request<InitiativeData>('/api/v3/initiatives'),
    ])
    if (!current(stamp)) return
    if (result[0].status === 'fulfilled') {
      tasks.value = result[0].value.tasks || result[0].value.items || []
      error.value = ''
      if (!selectedId.value) {
        const first = tasks.value.find((t) => isWorking(t)) || tasks.value[0]
        if (first) await selectTask(first.task_id)
      }
    } else error.value = 'Поручения пока недоступны.'
    if (!current(stamp)) return
    if (result[1].status === 'fulfilled')
      approvals.value = result[1].value.approvals || []
    if (result[2].status === 'fulfilled') initiatives.value = result[2].value
  }
  function schedulePoll() {
    clearTimeout(poll)
    if (!connected.value || disposed) return
    poll = setTimeout(async () => {
      if (!document.hidden && panel.value !== 'advanced') {
        await refresh()
        if (selectedId.value) await loadTask(selectedId.value, selection, true)
      }
      schedulePoll()
    }, 5000)
  }
  async function selectTask(id: string, open = false) {
    if (open) panel.value = 'task'
    selection++
    selectedId.value = id
    detail.value = null
    durable.value = null
    activity.value = []
    messages.value = []
    toolCalls.value = []
    pendingControl.value = null
    if (!connected.value) return
    await loadTask(id, selection)
  }
  async function loadTask(id: string, selected: number, background = false) {
    const stamp = epoch.value
    if (!background) loadingTask.value = true
    const valid = () =>
      current(stamp) && selected === selection && id === selectedId.value
    const result = await Promise.allSettled([
      request<TaskDetail>(`/api/v2/tasks/${encodeURIComponent(id)}`),
      request<{ task: DurableTask }>(`/api/v3/tasks/${encodeURIComponent(id)}`),
      request<{ activity: Activity[] }>(
        `/api/v2/tasks/${encodeURIComponent(id)}/activity`,
      ),
    ])
    if (!valid()) return
    if (result[0].status === 'fulfilled') detail.value = result[0].value
    if (result[1].status === 'fulfilled') {
      durable.value = result[1].value.task
      sessionKey.value = selectedTask.value?.session_key || sessionKey.value
    }
    if (result[2].status === 'fulfilled')
      activity.value = result[2].value.activity || []
    const activeRun = selectedTask.value?.run_id
    if (activeRun) {
      try {
        const transcript = await request<{
          messages: Message[]
          tool_calls?: TranscriptToolCall[]
        }>(`/api/v1/runs/${encodeURIComponent(activeRun)}/transcript`)
        if (valid() && selectedTask.value?.run_id === activeRun) {
          messages.value = transcript.messages || []
          toolCalls.value = transcript.tool_calls || []
        }
      } catch {
        /* Keep the last acknowledged transcript for this exact run. */
      }
    }
    if (valid() && !background) loadingTask.value = false
  }
  const refreshTask = () => loadTask(selectedId.value, selection, true)
  async function loadMemory(query = memoryQuery.value, more = false) {
    const stamp = epoch.value,
      version = ++memorySearch
    memoryQuery.value = query
    memoryBusy.value = true
    memoryError.value = ''
    if (!more) memories.value = []
    try {
      const offset = more ? memories.value.length : 0
      const result = query.trim()
        ? await request<MemoryPage>('/api/v4/memory/search', {
            query: query.trim(),
            limit: 6,
            offset,
          })
        : await request<MemoryPage>(
            `/api/v4/memory/browse?limit=6&offset=${offset}`,
          )
      if (
        !current(stamp) ||
        version !== memorySearch ||
        query !== memoryQuery.value
      )
        return
      memories.value = more
        ? [...memories.value, ...result.items]
        : result.items
      memoryHasMore.value = result.has_more
      memoryMode.value = result.mode
      memoryDiagnostics.value = result.diagnostics || []
    } catch {
      if (
        current(stamp) &&
        version === memorySearch &&
        query === memoryQuery.value
      )
        memoryError.value = 'Память пока недоступна. Попробуйте ещё раз.'
    } finally {
      if (
        current(stamp) &&
        version === memorySearch &&
        query === memoryQuery.value
      )
        memoryBusy.value = false
    }
  }
  async function inspectMemory(id: string) {
    const stamp = epoch.value,
      version = ++memorySelection
    inspection.value = null
    panel.value = 'memory'
    memoryError.value = ''
    if (!connected.value) return
    try {
      const loaded = await request<Inspection>(
        `/api/v4/memory/source?source_id=${encodeURIComponent(id)}`,
      )
      if (
        current(stamp) &&
        version === memorySelection &&
        loaded.source.source_id === id
      )
        inspection.value = loaded
    } catch {
      if (current(stamp) && version === memorySelection)
        memoryError.value = 'Этот источник недоступен или изменился.'
    }
  }
  function clearInspection() {
    memorySelection++
    inspection.value = null
  }
  async function saveMemory(title: string, body: string) {
    if (busy.value) return
    const stamp = epoch.value,
      version = memorySelection,
      existing = inspection.value?.source
    if (!existing && !pendingNote.value)
      pendingNote.value = { title, body, delivery: newID(), epoch: stamp }
    const note = pendingNote.value
    busy.value = true
    memoryError.value = ''
    try {
      if (existing)
        await request('/api/v4/memory/correct', {
          source_id: existing.source_id,
          expected_revision: existing.revision,
          title,
          body,
          author: existing.author || '',
          event_time: existing.event_time || null,
          review_legacy: false,
        })
      else if (note?.epoch === stamp) {
        await request(
          '/api/v4/memory/notes',
          { title: note.title, body: note.body },
          { delivery: note.delivery },
        )
        if (current(stamp)) pendingNote.value = null
      }
      if (!current(stamp) || version !== memorySelection) return
      clearInspection()
      panel.value = ''
      await loadMemory('')
    } catch {
      if (current(stamp))
        memoryError.value =
          'Запись не подтверждена. Проверьте текущую версию и повторите.'
    } finally {
      if (current(stamp)) busy.value = false
    }
  }
  async function hideMemory(action: 'revoke' | 'delete') {
    const source = inspection.value?.source
    if (!source) return
    const stamp = epoch.value,
      version = memorySelection
    busy.value = true
    try {
      await request('/api/v4/memory/delete', {
        source_id: source.source_id,
        expected_revision: source.revision,
        action,
      })
      if (current(stamp) && version === memorySelection) {
        clearInspection()
        panel.value = ''
        await loadMemory()
      }
    } catch {
      if (current(stamp))
        memoryError.value = 'Изменение не подтверждено. Обновите источник.'
    } finally {
      if (current(stamp)) busy.value = false
    }
  }
  async function control(kind: string, retry = false) {
    if (!durable.value || busy.value) return
    const stamp = epoch.value,
      version = selection
    const pending = retry
      ? pendingControl.value
      : {
          path: `/api/v3/tasks/${encodeURIComponent(durable.value.task_id)}/control`,
          body: { kind, expected_revision: durable.value.revision },
          delivery: newID(),
          epoch: stamp,
        }
    if (!pending || pending.epoch !== stamp) return
    pendingControl.value = pending
    busy.value = true
    error.value = ''
    try {
      await request(pending.path, pending.body, { delivery: pending.delivery })
      if (!current(stamp) || version !== selection) return
      pendingControl.value = null
      await refresh()
      if (current(stamp) && version === selection)
        await loadTask(selectedId.value, selection)
    } catch (cause) {
      if (current(stamp) && version === selection) {
        error.value =
          cause instanceof Error && cause.message === 'version_changed'
            ? 'Версия поручения изменилась. Обновите его перед действием.'
            : 'Подтверждение не получено. Повтор отправит то же действие.'
      }
    } finally {
      if (current(stamp)) busy.value = false
    }
  }
  async function decideApproval(id: string, decision: 'approve' | 'reject') {
    const stamp = epoch.value
    busy.value = true
    error.value = ''
    try {
      await request(
        `/api/v2/approvals/${encodeURIComponent(id)}/${decision}`,
        {},
      )
      if (current(stamp)) await refresh()
    } catch {
      if (current(stamp))
        error.value = 'Решение не подтверждено. Обновите запрос перед повтором.'
    } finally {
      if (current(stamp)) busy.value = false
    }
  }
  async function send(retry = false) {
    if (
      sending.value ||
      !connected.value ||
      (!retry && (!draft.value.trim() || pendingIngress.value))
    )
      return
    const stamp = epoch.value
    if (!sessionKey.value) sessionKey.value = `web:owner:${newID()}`
    const delivery = newID()
    const pending = retry
      ? pendingIngress.value
      : {
          path: '/api/v1/events',
          delivery,
          epoch: stamp,
          body: {
            event_id: delivery,
            idempotency_key: delivery,
            event_type: 'user_message',
            source: 'web',
            session_key: sessionKey.value,
            payload: { text: draft.value.trim() },
          },
        }
    if (!pending || pending.epoch !== stamp) return
    pendingIngress.value = pending
    sending.value = true
    error.value = ''
    try {
      const accepted = await request<
        Receipt & {
          accepted?: boolean
          receipt_status?: string
          assistant_response?: string
        }
      >(pending.path, pending.body, { delivery: pending.delivery })
      if (!current(stamp)) return
      receipt.value = {
        ...accepted,
        status: accepted.receipt_status || accepted.status || 'queued',
      }
      pendingIngress.value = null
      draft.value = ''
      if (accepted.task_id) await selectTask(accepted.task_id)
      if (accepted.input_id && current(stamp)) pollReceipt(accepted.input_id)
      await refresh()
    } catch {
      if (current(stamp))
        error.value =
          'Подтверждение не получено. Текст сохранён; повтор безопасно использует тот же запрос.'
    } finally {
      if (current(stamp)) sending.value = false
    }
  }
  function pollReceipt(id: string) {
    const stamp = epoch.value
    clearTimeout(receiptPoll)
    receiptPoll = setTimeout(async () => {
      if (!current(stamp) || receipt.value?.input_id !== id) return
      try {
        const next = await request<Receipt>(
          `/api/v3/inputs/${encodeURIComponent(id)}`,
        )
        if (!current(stamp) || receipt.value?.input_id !== id) return
        receipt.value = next
        if (next.task_id && selectedId.value !== next.task_id)
          await selectTask(next.task_id)
        if (!current(stamp)) return
        await refresh()
        if (
          !['completed', 'failed', 'cancelled', 'expired'].includes(next.status)
        )
          pollReceipt(id)
      } catch {
        if (current(stamp))
          error.value = 'Запрос принят, но его состояние сейчас недоступно.'
      }
    }, 1500)
  }
  async function openReceipt(id: string) {
    const stamp = epoch.value
    const next = await request<Receipt>(
      `/api/v3/inputs/${encodeURIComponent(id)}`,
    )
    if (!current(stamp)) return
    receipt.value = next
    if (next.task_id) await selectTask(next.task_id)
    if (
      current(stamp) &&
      !['completed', 'failed', 'cancelled', 'expired'].includes(next.status)
    )
      pollReceipt(id)
  }
  onScopeDispose(() => {
    disposed = true
    resetPrivate()
    token.value = ''
  })
  watch(
    runId,
    () => {
      messages.value = []
      activity.value = []
      toolCalls.value = []
    },
    { flush: 'sync' },
  )
  return {
    token,
    connected,
    connecting,
    connectionError,
    epoch,
    tasks,
    approvals,
    initiatives,
    selectedId,
    selectedTask,
    detail,
    durable,
    activity,
    messages,
    toolCalls,
    conversation,
    runId,
    working,
    calendar,
    notices,
    selectedNotice,
    panel,
    advanced,
    error,
    busy,
    loadingTask,
    memories,
    memoryMode,
    memoryDiagnostics,
    memoryHasMore,
    memoryQuery,
    memoryBusy,
    memoryError,
    inspection,
    draft,
    receipt,
    sending,
    pendingIngress,
    pendingControl,
    pendingNote,
    sessionKey,
    motion,
    quiet,
    noticeHidden,
    connect,
    refresh,
    selectTask,
    refreshTask,
    loadTask,
    request,
    loadMemory,
    inspectMemory,
    clearInspection,
    saveMemory,
    hideMemory,
    control,
    decideApproval,
    send,
    openReceipt,
    resetPrivate,
  }
})
