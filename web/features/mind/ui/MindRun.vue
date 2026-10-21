<template>
  <div class="mind-log" aria-label="Последние действия">
    <TransitionGroup name="mind-entry" tag="div"
      ><details v-for="(item, index) in recent" :key="item.id">
        <summary>
          <time>{{ timeLabel(item.at) }}</time
          ><MindIcon :name="item.icon" /><MindTypewriter
            :text="item.text"
            :live="index === recent.length - 1 && item.live"
            :motion="motion"
          />
        </summary>
        <p v-if="item.detail">{{ item.detail }}</p>
      </details></TransitionGroup
    >
    <p v-if="!recent.length" class="mind-muted">
      {{
        stream.error.value ? 'Связь с журналом прервана.' : 'Ожидаю событий.'
      }}
    </p>
    <span class="sr-only" role="status">{{
      stream.connected.value
        ? 'Журнал подключён'
        : stream.ended.value
          ? 'Запуск завершён'
          : 'Журнал подключается'
    }}</span>
  </div>
</template>
<script setup lang="ts">
import { useRunEvents } from '~/composables/useRunEvents'
import type { Activity } from '../model/types'
import { timeLabel } from '../model/presentation'
import MindIcon from './MindIcon.vue'
import MindTypewriter from './MindTypewriter.vue'
const props = defineProps<{
  runId: string
  activity: Activity[]
  motion: boolean
}>()
const emit = defineEmits<{ ended: []; connection: [connected: boolean] }>()
const stream = useRunEvents(props.runId)
watch(stream.ended, (value) => {
  if (value) emit('ended')
})
watch(stream.connected, (value) => emit('connection', value), {
  immediate: true,
})
const labels: Record<string, string> = {
  memory_loaded: 'Вспомнил нужное',
  tools_loaded: 'Подготовил инструменты',
  prompt_assembled: 'Подготовил запрос',
  assistant_delta: 'Получаю ответ',
  assistant_final: 'Ответ модели получен',
  tool_started: 'Использую инструмент',
  tool_completed: 'Инструмент вернул результат',
  approval_requested: 'Жду вашего разрешения',
  approval_resolved: 'Ваше решение принято',
  run_error: 'Не удалось продолжить',
  run_completed: 'Запуск завершён',
  state_transition: 'Перешёл к следующему шагу',
}
const recent = computed(() => {
  const rows: {
    id: string
    at: string
    text: string
    detail: string
    icon: string
    live: boolean
  }[] = props.activity.map((a) => ({
    id: `activity-${a.activity_id}`,
    at: a.created_at,
    text: a.title,
    detail: a.summary,
    icon: /memory/.test(a.activity_type) ? 'stack' : 'event',
    live: false,
  }))
  for (const event of stream.events.value) {
    const tool =
      typeof event.payload.tool_name === 'string' ? event.payload.tool_name : ''
    const text = labels[event.event_type] || 'Новое событие'
    if (
      event.event_type === 'assistant_delta' &&
      rows.at(-1)?.id.startsWith('delta-')
    ) {
      rows[rows.length - 1].at = event.timestamp
      continue
    }
    rows.push({
      id:
        event.event_type === 'assistant_delta'
          ? `delta-${event.event_id}`
          : event.event_id,
      at: event.timestamp,
      text: tool ? `${text}: ${tool}` : text,
      detail: '',
      icon: /memory/.test(event.event_type)
        ? 'stack'
        : /tool/.test(event.event_type)
          ? 'document'
          : 'message',
      live: true,
    })
  }
  return rows.sort((a, b) => a.at.localeCompare(b.at)).slice(-3)
})
</script>
