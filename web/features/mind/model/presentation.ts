import type { CalendarItem, Message, Source, Task } from './types'

export const newID = () =>
  [...crypto.getRandomValues(new Uint8Array(16))]
    .map((x) => x.toString(16).padStart(2, '0'))
    .join('')
export function dayKey(value: Date | string): string {
  const date = typeof value === 'string' ? new Date(value) : value
  if (!Number.isFinite(date.getTime())) return ''
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}
export function timeLabel(value?: string): string {
  if (!value || !Number.isFinite(new Date(value).getTime())) return ''
  return new Intl.DateTimeFormat('ru-RU', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}
export function dateLabel(value?: string): string {
  if (!value || !Number.isFinite(new Date(value).getTime())) return ''
  return dayKey(value) === dayKey(new Date())
    ? timeLabel(value)
    : new Intl.DateTimeFormat('ru-RU', {
        day: 'numeric',
        month: 'short',
      }).format(new Date(value))
}
const labels: Record<string, string> = {
  active: 'В работе',
  in_progress: 'В работе',
  running: 'В работе',
  queued: 'В очереди',
  waiting_for_user: 'Нужно уточнение',
  waiting_for_approval: 'Ждёт разрешения',
  waiting_for_reply_in_telegram: 'Ждёт ответа в Telegram',
  blocked: 'Нужна помощь',
  paused: 'На паузе',
  cancelled: 'Остановлено',
  completed: 'Завершено',
  failed: 'Ошибка',
  completed_with_issues: 'Завершено с замечаниями',
  scheduled: 'Запланировано',
}
export const statusLabel = (status: string) =>
  labels[status] || status || 'Состояние неизвестно'
export const taskStatus = (task: Task) => task.task_status || task.status
export const taskTitle = (task: Task) =>
  task.goal?.trim() || `Поручение ${task.task_id.slice(0, 8)}`
export function isWorking(task: Task | null): boolean {
  if (
    !task ||
    task.needs_user_action ||
    !['active', 'running', 'in_progress'].includes(taskStatus(task))
  )
    return false
  return !/WAITING|PAUSED|COMPLETED|FAILED|CANCELLED|QUEUED/i.test(
    task.run_state || '',
  )
}
export function calendarTasks(tasks: Task[]): CalendarItem[] {
  return tasks
    .filter((task) => !!(task.started_at || task.updated_at))
    .map((task) => ({
      id: task.task_id,
      taskId: task.task_id,
      title: taskTitle(task),
      at: task.started_at || task.updated_at,
      status: taskStatus(task),
      done: task.result_verified === true,
      source: 'task',
      description: statusLabel(taskStatus(task)),
    }))
}
export function publicMessage(message: Message): boolean {
  if (!['user', 'assistant'].includes(message.role)) return false
  if (message.visibility && !['public', 'owner'].includes(message.visibility))
    return false
  if (
    message.kind &&
    ![
      'publication',
      'owner_input',
      'interaction_response',
      'user_message',
    ].includes(message.kind)
  )
    return false
  if (message.role === 'assistant' && message.published === false) return false
  if (message.metadata_json) {
    try {
      const meta = JSON.parse(message.metadata_json) as {
        kind?: string
        visibility?: string
        published?: boolean
      }
      if (
        meta.kind === 'attempt_final' ||
        meta.visibility === 'internal' ||
        meta.published === false
      )
        return false
    } catch {
      return false
    }
  }
  return true
}
export function memoryIcon(source: Source): string {
  if (/message|statement|profile/.test(source.source_kind)) return 'message'
  if (/event|observation|notification/.test(source.source_kind)) return 'event'
  return 'document'
}
export function memoryOrigin(source: Source): string {
  return (
    source.author ||
    (
      {
        owner_statement: 'Ваше сообщение',
        document: 'Документ',
        notification: 'Уведомление',
        task_result: 'Результат поручения',
      } as Record<string, string>
    )[source.source_kind] ||
    'Сохранённый источник'
  )
}
