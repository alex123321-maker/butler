<template>
  <div class="page">
    <div class="page-header">
      <NuxtLink v-if="data?.run" :to="`/sessions/${encodeURIComponent(data.run.session_key)}`" class="back-link">&larr; Session</NuxtLink>
      <NuxtLink v-else to="/sessions" class="back-link">&larr; Sessions</NuxtLink>
      <h2 class="page-title">Run Detail</h2>
    </div>

    <div v-if="pending" class="placeholder-text">Loading run transcript...</div>
    <div v-else-if="error" class="placeholder-text">Failed to load run.</div>
    <template v-else-if="data">
      <!-- Run info card -->
      <div class="card run-info">
        <div class="info-row"><span class="info-label">Run ID</span><span class="mono">{{ data.run.run_id }}</span></div>
        <div class="info-row"><span class="info-label">State</span><span :class="stateBadgeClass(data.run.current_state)" class="state-badge">{{ data.run.current_state }}</span></div>
        <div class="info-row"><span class="info-label">Provider</span><span>{{ data.run.model_provider }}</span></div>
        <div class="info-row"><span class="info-label">Autonomy</span><span>{{ data.run.autonomy_mode }}</span></div>
        <div class="info-row"><span class="info-label">Started</span><span>{{ formatTime(data.run.started_at) }}</span></div>
        <div class="info-row"><span class="info-label">Finished</span><span>{{ data.run.finished_at ? formatTime(data.run.finished_at) : '—' }}</span></div>
        <div v-if="data.run.error_message" class="info-row"><span class="info-label">Error</span><span class="error-text">{{ data.run.error_type }}: {{ data.run.error_message }}</span></div>
      </div>

      <!-- Transcript -->
      <h3 class="section-title">Transcript</h3>

      <div v-if="data.messages.length === 0 && data.tool_calls.length === 0" class="placeholder-text">No transcript entries.</div>

      <div v-else class="transcript">
        <div v-for="entry in timeline" :key="entry.id" :class="['transcript-entry', `transcript-entry--${entry.type}`]">
          <!-- Message -->
          <template v-if="entry.type === 'message'">
            <div class="entry-header">
              <span :class="['role-badge', `role-badge--${entry.role}`]">{{ entry.role }}</span>
              <span class="entry-time">{{ formatTime(entry.time) }}</span>
            </div>
            <div class="entry-content">{{ entry.content }}</div>
          </template>

          <!-- Tool call -->
          <template v-if="entry.type === 'tool_call'">
            <div class="entry-header">
              <span class="role-badge role-badge--tool">{{ entry.toolName }}</span>
              <span :class="stateBadgeClass(entry.status || '')" class="state-badge">{{ entry.status }}</span>
              <span class="entry-time">{{ formatTime(entry.time) }}</span>
            </div>
            <details class="tool-details">
              <summary>Arguments &amp; Result</summary>
              <div class="code-block"><strong>Args:</strong> {{ entry.argsJson }}</div>
              <div class="code-block"><strong>Result:</strong> {{ entry.resultJson }}</div>
              <div v-if="entry.errorJson && entry.errorJson !== '{}'" class="code-block error-text"><strong>Error:</strong> {{ entry.errorJson }}</div>
            </details>
          </template>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { useRunTranscript, type TranscriptMessage, type TranscriptToolCall } from '~/composables/useApi'

const route = useRoute()
const runId = route.params.id as string

useHead({ title: `Run ${runId} — Butler` })

const { data, pending, error } = useRunTranscript(runId)

interface TimelineEntry {
  id: string
  type: 'message' | 'tool_call'
  time: string
  role?: string
  content?: string
  toolName?: string
  argsJson?: string
  resultJson?: string
  errorJson?: string
  status?: string
}

const timeline = computed<TimelineEntry[]>(() => {
  if (!data.value) return []

  const entries: TimelineEntry[] = []

  for (const m of data.value.messages) {
    entries.push({
      id: m.message_id,
      type: 'message',
      time: m.created_at,
      role: m.role,
      content: m.content,
    })
  }

  for (const tc of data.value.tool_calls) {
    entries.push({
      id: tc.tool_call_id,
      type: 'tool_call',
      time: tc.started_at,
      toolName: tc.tool_name,
      argsJson: tc.args_json,
      resultJson: tc.result_json,
      errorJson: tc.error_json,
      status: tc.status,
    })
  }

  entries.sort((a, b) => new Date(a.time).getTime() - new Date(b.time).getTime())
  return entries
})

function formatTime(iso: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString()
}

function stateBadgeClass(state: string): string {
  if (state === 'completed') return 'state-badge--completed'
  if (state === 'failed' || state === 'timed_out') return 'state-badge--failed'
  if (state === 'model_running' || state === 'tool_running') return 'state-badge--running'
  return 'state-badge--pending'
}
</script>

<style scoped>
.page-header {
  margin-bottom: 24px;
}

.back-link {
  font-size: 13px;
  color: var(--color-text-muted);
  display: inline-block;
  margin-bottom: 8px;
}

.back-link:hover {
  color: var(--color-primary);
}

.run-info {
  margin-bottom: 32px;
}

.info-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 0;
  font-size: 14px;
}

.info-label {
  color: var(--color-text-muted);
  font-weight: 600;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.3px;
  min-width: 120px;
}

.mono {
  font-family: monospace;
  font-size: 12px;
}

.error-text {
  color: var(--color-danger);
}

.section-title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 16px;
}

.transcript {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.transcript-entry {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 8px;
  padding: 14px 16px;
}

.transcript-entry--tool_call {
  border-left: 3px solid var(--color-warning);
}

.entry-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}

.entry-time {
  font-size: 11px;
  color: var(--color-text-muted);
  margin-left: auto;
}

.entry-content {
  font-size: 14px;
  white-space: pre-wrap;
  line-height: 1.6;
}

.role-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

.role-badge--user {
  background: rgba(79, 140, 255, 0.15);
  color: var(--color-primary);
}

.role-badge--assistant {
  background: rgba(52, 211, 153, 0.15);
  color: var(--color-success);
}

.role-badge--system {
  background: rgba(139, 143, 163, 0.15);
  color: var(--color-text-muted);
}

.role-badge--tool {
  background: rgba(251, 191, 36, 0.15);
  color: var(--color-warning);
}

.tool-details {
  margin-top: 8px;
}

.tool-details summary {
  font-size: 12px;
  color: var(--color-text-muted);
  cursor: pointer;
}

.code-block {
  margin-top: 6px;
  padding: 8px 10px;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 4px;
  font-family: monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
