<template>
  <p v-if="pending" class="mind-muted">Открываю историю…</p>
  <p v-else-if="error" class="mind-error" role="alert">{{ error }}</p>
  <template v-else-if="kind === 'session'"
    ><p class="mind-muted">{{ session?.session.channel }} · {{ id }}</p>
    <NuxtLink
      v-for="attempt in session?.runs || []"
      :key="attempt.run_id"
      class="mind-panel-row"
      :to="`/runs/${encodeURIComponent(attempt.run_id)}`"
      ><span
        >{{ statusLabel(attempt.current_state)
        }}<small>{{ dateLabel(attempt.started_at) }}</small></span
      ><span>↗</span></NuxtLink
    >
    <p v-if="!session?.runs.length" class="mind-muted">
      Попыток пока нет.
    </p></template
  >
  <template v-else-if="run"
    ><NuxtLink
      class="mind-text-button"
      :to="`/sessions/${encodeURIComponent(run.run.session_key)}`"
      >Разговор ↗</NuxtLink
    >
    <p class="mind-muted">
      {{ statusLabel(run.run.current_state) }} ·
      {{ dateLabel(run.run.started_at) }}
    </p>
    <MindRun
      :key="id"
      :run-id="id"
      :activity="[]"
      :tools="run.tool_calls || []"
      :motion="store.motion"
      full
      @result="refreshTools"
    />
    <details>
      <summary>Записанный разговор</summary>
      <p
        v-for="message in visibleMessages"
        :key="message.message_id"
        class="mind-readable"
      >
        {{ message.role === 'user' ? 'Вы' : 'Батлер' }}: {{ message.content }}
      </p>
      <p v-if="!visibleMessages.length" class="mind-muted">
        Опубликованных сообщений нет.
      </p>
    </details>
  </template>
</template>
<script setup lang="ts">
import type {
  RunRecord,
  SessionRecord,
  TranscriptToolCall,
} from '~/composables/useApi'
import type { Message } from '../model/types'
import { useMind } from '../model/store'
import { publicMessage, dateLabel, statusLabel } from '../model/presentation'
import MindRun from './MindRun.vue'
const props = defineProps<{ kind: 'run' | 'session'; id: string }>(),
  store = useMind()
const session = ref<{ session: SessionRecord; runs: RunRecord[] } | null>(null),
  run = ref<{
    run: RunRecord
    messages: Message[]
    tool_calls: TranscriptToolCall[]
  } | null>(null),
  pending = ref(true),
  error = ref('')
const visibleMessages = computed(
  () => run.value?.messages.filter(publicMessage) || [],
)
let version = 0
async function refreshTools() {
  const current = version,
    id = props.id
  try {
    const data = await store.request<{ tool_calls?: TranscriptToolCall[] }>(
      `/api/v1/runs/${encodeURIComponent(id)}/transcript`,
    )
    if (current === version && run.value)
      run.value.tool_calls = data.tool_calls || []
  } catch {
    /* Keep the last acknowledged calls for this run. */
  }
}
onUnmounted(() => {
  version++
})
watch(
  () => [props.kind, props.id, store.epoch] as const,
  async ([kind, id], _, cleanup) => {
    const current = ++version
    let closed = false
    cleanup(() => {
      closed = true
    })
    pending.value = true
    error.value = ''
    session.value = null
    run.value = null
    try {
      if (kind === 'session') {
        const data = await store.request<{
          session: SessionRecord
          runs: RunRecord[]
        }>(`/api/v1/sessions/${encodeURIComponent(id)}`)
        if (!closed && current === version) session.value = data
      } else {
        const data = await store.request<{
          run: RunRecord
          messages: Message[]
          tool_calls: TranscriptToolCall[]
        }>(`/api/v1/runs/${encodeURIComponent(id)}/transcript`)
        if (!closed && current === version) run.value = data
      }
    } catch {
      if (!closed && current === version)
        error.value = 'История сейчас недоступна.'
    } finally {
      if (!closed && current === version) pending.value = false
    }
  },
  { immediate: true },
)
</script>
