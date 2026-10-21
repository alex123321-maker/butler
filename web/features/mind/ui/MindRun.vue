<template>
  <MindEventLog
    :rows="rows"
    :motion="motion"
    :compact="!full"
    :working="stream.connected.value && !stream.ended.value"
    :empty="
      stream.error.value ? 'Связь с журналом прервана.' : 'Ожидаю событий.'
    "
  />
  <span class="sr-only" role="status">{{
    stream.connected.value
      ? 'Журнал подключён'
      : stream.ended.value
        ? 'Запуск завершён'
        : 'Журнал подключается'
  }}</span>
</template>
<script setup lang="ts">
import type { TranscriptToolCall } from '~/composables/useApi'
import { useRunEvents, type RunEvent } from '~/composables/useRunEvents'
import type { Activity } from '../model/types'
import { runLogRows } from '../model/run-log'
import MindEventLog from './MindEventLog.vue'
const props = defineProps<{
  runId: string
  activity: Activity[]
  tools?: TranscriptToolCall[]
  motion: boolean
  full?: boolean
}>()
const emit = defineEmits<{
  ended: []
  connection: [connected: boolean]
  events: [events: RunEvent[]]
  result: []
}>()
const stream = useRunEvents(props.runId)
watch(stream.ended, (value) => {
  if (value) emit('ended')
})
watch(stream.connected, (value) => emit('connection', value), {
  immediate: true,
})
let lastResults = ''
watch(stream.events, (events) => {
  emit('events', events)
  const results = events
    .filter((e) =>
      ['tool_completed', 'run_error', 'run_completed'].includes(e.event_type),
    )
    .map((e) => e.event_id || e.timestamp)
    .join('|')
  if (results !== lastResults) {
    lastResults = results
    emit('result')
  }
})
const rows = computed(() =>
  runLogRows(
    props.runId,
    props.activity,
    stream.events.value,
    props.tools || [],
  ),
)
</script>
