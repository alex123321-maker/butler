export type Tone = 'default' | 'success' | 'warning' | 'error' | 'info'

const statusLabels: Record<string, string> = {
  in_progress: 'In progress',
  waiting_for_approval: 'Waiting for approval',
  waiting_for_reply_in_telegram: 'Waiting for your reply',
  completed: 'Completed',
  completed_with_issues: 'Completed with issues',
  failed: 'Failed',
  cancelled: 'Cancelled',
}

const waitingReasonLabels: Record<string, string> = {
  approval_required: 'Approval required',
  telegram_reply_required: 'Reply needed in Telegram',
  user_reply_required: 'Waiting for your reply',
}

export function formatTaskStatus(status: string): string {
  return statusLabels[status] ?? startCase(status || 'unknown')
}

export function formatWaitingReason(reason: string): string {
  if (!reason) {
    return 'No blocker'
  }
  return waitingReasonLabels[reason] ?? startCase(reason)
}

export function taskTone(status: string): Tone {
  if (status.includes('failed')) {
    return 'error'
  }
  if (status.includes('approval') || status.includes('waiting')) {
    return 'warning'
  }
  if (status.includes('completed')) {
    return 'success'
  }
  if (status.includes('progress')) {
    return 'info'
  }
  return 'default'
}

export function describeTaskState(task: {
  status: string
  waiting_reason?: string
  needs_user_action?: boolean
  user_action_channel?: string
  source_channel?: string
}): string {
  if (task.status === 'waiting_for_reply_in_telegram') {
    return 'The assistant is waiting for you in Telegram before it can continue.'
  }
  if (task.status === 'waiting_for_approval') {
    return 'The assistant is paused and waiting for your approval.'
  }
  if (task.needs_user_action) {
    const channel = task.user_action_channel ? ` in ${formatChannel(task.user_action_channel)}` : ''
    return `Your action is needed${channel}.`
  }
  if (task.status === 'completed_with_issues') {
    return 'The task finished, but there were issues worth reviewing.'
  }
  if (task.status === 'completed') {
    return 'The task finished successfully.'
  }
  if (task.status === 'failed') {
    return 'The task failed and needs attention.'
  }
  return 'The assistant is currently working on this task.'
}

export function formatTaskId(taskId: string): string {
  if (!taskId) {
    return 'Task'
  }
  return `Task ${taskId}`
}

export function formatChannel(channel: string): string {
  if (!channel) {
    return 'the system'
  }
  return startCase(channel)
}

export function formatDateTime(value: string | null | undefined): string {
  if (!value) {
    return '-'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return date.toLocaleString()
}

function startCase(value: string): string {
  return value
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/\b\w/g, (char) => char.toUpperCase())
}
