import type { TranscriptToolCall } from '~/composables/useApi'
import type { RunEvent } from '~/composables/useRunEvents'
import type { Activity } from './types'

export interface LogRow {
  id: string
  at: string
  type: string
  label: string
  text: string
  pending: boolean
  failed: boolean
  duration: string
  sections: { icon: string; label: string; text: string }[]
}

export const eventLabels: Record<string, string> = {
  state_transition: 'Смена состояния',
  memory_loaded: 'Память загружена',
  tools_loaded: 'Инструменты подготовлены',
  prompt_assembled: 'Запрос подготовлен',
  assistant_delta: 'Модель отвечает',
  assistant_final: 'Ответ модели сохранён',
  tool_started: 'Инструмент выполняется',
  tool_completed: 'Инструмент вернул результат',
  tool_failed: 'Ошибка инструмента',
  approval_requested: 'Запрошено разрешение',
  approval_resolved: 'Решение получено',
  run_error: 'Ошибка запуска',
  run_completed: 'Запуск завершён',
}
const aliases: Record<string, string> = {
  task_received: 'memory_loaded',
  model_started: 'prompt_assembled',
  task_completed: 'run_completed',
  task_failed: 'run_error',
}
function parse(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}
function masked(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(masked)
  if (value && typeof value === 'object')
    return Object.fromEntries(
      Object.entries(value)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([key, entry]) => [
          key,
          /^(password|passwd|secret|token|access_token|refresh_token|api_key|authorization|cookie|set-cookie|credential_value)$/i.test(
            key,
          )
            ? '••••'
            : masked(entry),
        ]),
    )
  if (typeof value === 'string')
    return value.replace(/\bBearer\s+[\w.+~/-]+=*/gi, 'Bearer ••••')
  return value
}
function pretty(value: unknown): string {
  const safe = masked(typeof value === 'string' ? parse(value) : value)
  return typeof safe === 'string' ? safe : JSON.stringify(safe, null, 2) || ''
}
function record(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}
}
const string = (value: unknown) => (typeof value === 'string' ? value : '')
const isFailure = (status: string) =>
  /failed|error|denied|rejected|unknown|stopped/.test(status)
function eventText(type: string, payload: Record<string, unknown>) {
  if (type.startsWith('tool_')) return string(payload.tool_name)
  if (type === 'memory_loaded' && typeof payload.bundle_count === 'number')
    return `${payload.bundle_count}`
  if (type === 'tools_loaded' && typeof payload.tool_count === 'number')
    return `${payload.tool_count}`
  if (type === 'run_error')
    return (
      string(payload.error_message) ||
      string(payload.error) ||
      string(payload.message)
    )
  return ''
}
export function runLogRows(
  runId: string,
  activity: Activity[],
  events: RunEvent[],
  tools: TranscriptToolCall[],
): LogRow[] {
  const rows = new Map<string, LogRow>()
  const observed = activity
    .filter((a) => !a.run_id || a.run_id === runId)
    .map((a) => ({
      id: `activity-${a.activity_id}`,
      type: aliases[a.activity_type] || a.activity_type,
      at: a.created_at,
      payload: record(parse(a.details_json || '{}')),
    }))
  const live = events.filter((e) => e.run_id === runId)
  // Activity DTO timestamps have second precision; SSE retains sub-seconds.
  // Consume one persisted twin per live event, preserving genuine repetitions.
  for (const event of live) {
    const twin = observed.findIndex(
      (a) =>
        a.type === event.event_type &&
        Math.floor(Date.parse(a.at) / 1000) ===
          Math.floor(Date.parse(event.timestamp) / 1000) &&
        pretty(a.payload) === pretty(event.payload),
    )
    if (twin !== -1) observed.splice(twin, 1)
  }
  observed.push(
    ...live.map((e) => ({
      id: e.event_id || `${e.event_type}-${e.timestamp}`,
      type: e.event_type,
      at: e.timestamp,
      payload: e.payload,
    })),
  )
  observed.sort((a, b) => Date.parse(a.at) - Date.parse(b.at))
  let deltaID = ''
  for (const { id: eventID, type, at, payload } of observed) {
    const call = string(payload.tool_call_id),
      tool = type === 'tool_started' || type === 'tool_completed'
    if (type !== 'assistant_delta') deltaID = ''
    else if (!deltaID) deltaID = eventID
    const id = tool && call ? `tool-${call}` : deltaID || eventID
    // A delayed/replayed start must not replace an observed terminal result.
    if (
      tool &&
      type === 'tool_started' &&
      rows.has(id) &&
      !rows.get(id)?.pending
    )
      continue
    const failed =
      tool && (payload.has_error === true || isFailure(string(payload.status)))
    const displayType = failed ? 'tool_failed' : type
    const sections: LogRow['sections'] = []
    if (Object.keys(payload).length)
      sections.push({
        icon: 'document',
        label: 'Данные события',
        text: pretty(payload),
      })
    rows.set(id, {
      id,
      at,
      type: displayType,
      label: eventLabels[displayType] || type,
      text: pretty(eventText(type, payload)),
      pending: type === 'tool_started',
      failed,
      duration:
        typeof payload.duration_ms === 'number'
          ? `${payload.duration_ms} ms`
          : '',
      sections,
    })
  }
  for (const call of tools.filter((t) => t.run_id === runId)) {
    const id = `tool-${call.tool_call_id}`,
      previous = rows.get(id)
    const failed =
      isFailure(call.status) ||
      (!!call.error_json &&
        call.error_json !== '{}' &&
        call.error_json !== 'null')
    const pending =
      !call.finished_at &&
      !failed &&
      /^(requested|started|running|pending)$/.test(call.status)
    const type = failed
      ? 'tool_failed'
      : pending
        ? 'tool_started'
        : 'tool_completed'
    const duration = call.finished_at
      ? Date.parse(call.finished_at) - Date.parse(call.started_at)
      : NaN
    const sections: LogRow['sections'] = []
    if (call.args_json)
      sections.push({
        icon: 'input',
        label: 'Аргументы',
        text: pretty(call.args_json),
      })
    if (call.result_json)
      sections.push({
        icon: 'output',
        label: 'Результат инструмента',
        text: pretty(call.result_json),
      })
    if (
      call.error_json &&
      call.error_json !== '{}' &&
      call.error_json !== 'null'
    )
      sections.push({
        icon: 'tool_failed',
        label: 'Ошибка',
        text: pretty(call.error_json),
      })
    sections.push({
      icon: 'document',
      label: 'Данные вызова',
      text: pretty({
        tool_call_id: call.tool_call_id,
        status: call.status,
        runtime_target: call.runtime_target,
        started_at: call.started_at,
        finished_at: call.finished_at,
      }),
    })
    rows.set(id, {
      id,
      at: call.finished_at || previous?.at || call.started_at,
      type,
      label: eventLabels[type],
      text: call.tool_name,
      pending,
      failed,
      duration: Number.isFinite(duration)
        ? `${Math.max(0, duration)} ms`
        : previous?.duration || '',
      sections,
    })
  }
  return [...rows.values()].sort((a, b) => Date.parse(a.at) - Date.parse(b.at))
}
