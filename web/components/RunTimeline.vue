<template>
  <div class="run-timeline">
    <div class="timeline-header">
      <h3 class="timeline-title">Live Timeline</h3>
      <div class="timeline-status">
        <span v-if="connected" class="status-dot status-dot--live" />
        <span v-else-if="ended" class="status-dot status-dot--ended" />
        <span v-else class="status-dot status-dot--disconnected" />
        <span class="status-label">{{ statusLabel }}</span>
      </div>
    </div>

    <div v-if="displayEvents.length === 0 && !ended" class="timeline-empty">
      Waiting for events...
    </div>

    <div v-else class="timeline-list">
      <div
        v-for="item in displayEvents"
        :key="item.key"
        :class="['timeline-item', `timeline-item--${item.category}`]"
      >
        <div class="timeline-marker" :class="`timeline-marker--${item.category}`" />
        <div class="timeline-content">
          <div class="timeline-event-header">
            <span class="event-type-badge" :class="`event-type-badge--${item.category}`">
              {{ item.label }}
            </span>
            <span class="event-time">{{ item.time }}</span>
          </div>
          <div class="event-detail">{{ item.summary }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRunEvents, type RunEvent } from '~/composables/useRunEvents'

const props = defineProps<{
  runId: string
}>()

const { events, connected, ended } = useRunEvents(props.runId)

const statusLabel = computed(() => {
  if (connected.value) return 'Live'
  if (ended.value) return 'Ended'
  return 'Disconnected'
})

// --- Display model: collapse consecutive assistant_delta events ---

interface DisplayItem {
  key: string
  category: string
  label: string
  time: string
  summary: string
}

const displayEvents = computed<DisplayItem[]>(() => {
  const items: DisplayItem[] = []
  let deltaGroup: RunEvent[] = []

  function flushDeltas() {
    if (deltaGroup.length === 0) return
    const first = deltaGroup[0]
    const last = deltaGroup[deltaGroup.length - 1]
    const totalChars = deltaGroup.reduce(
      (sum, e) => sum + ((e.payload?.content_length as number) || 0),
      0,
    )
    const isLive = !ended.value && deltaGroup === currentDeltaGroup
    items.push({
      key: `delta-group-${first.event_id}`,
      category: 'model',
      label: isLive ? 'Streaming...' : 'Streamed',
      time: formatEventTime(last.timestamp),
      summary: `${deltaGroup.length} chunk(s), ${totalChars} chars total`,
    })
    deltaGroup = []
  }

  // Keep a reference to identify if deltas are still live (at the tail of events).
  let currentDeltaGroup: RunEvent[] = []
  let lastWasDelta = false

  for (const evt of events.value) {
    if (evt.event_type === 'assistant_delta') {
      if (!lastWasDelta) {
        // Start a new group.
        deltaGroup = []
        currentDeltaGroup = deltaGroup
      }
      deltaGroup.push(evt)
      currentDeltaGroup = deltaGroup
      lastWasDelta = true
    } else {
      if (lastWasDelta) {
        flushDeltas()
      }
      lastWasDelta = false
      items.push({
        key: evt.event_id,
        category: eventCategory(evt.event_type),
        label: eventLabel(evt.event_type),
        time: formatEventTime(evt.timestamp),
        summary: eventSummary(evt),
      })
    }
  }

  // Flush any trailing delta group (still streaming).
  if (lastWasDelta) {
    flushDeltas()
  }

  return items
})

// --- Helpers ---

function eventCategory(eventType: string): string {
  switch (eventType) {
    case 'state_transition': return 'state'
    case 'memory_loaded':
    case 'tools_loaded':
    case 'prompt_assembled': return 'prepare'
    case 'assistant_delta':
    case 'assistant_final': return 'model'
    case 'tool_started':
    case 'tool_completed': return 'tool'
    case 'approval_requested':
    case 'approval_resolved': return 'approval'
    case 'run_error': return 'error'
    case 'run_completed': return 'completed'
    default: return 'default'
  }
}

function eventLabel(eventType: string): string {
  const labels: Record<string, string> = {
    state_transition: 'State',
    memory_loaded: 'Memory',
    tools_loaded: 'Tools',
    prompt_assembled: 'Prompt',
    assistant_delta: 'Streaming',
    assistant_final: 'Response',
    tool_started: 'Tool Start',
    tool_completed: 'Tool Done',
    approval_requested: 'Approval',
    approval_resolved: 'Approved',
    run_error: 'Error',
    run_completed: 'Completed',
  }
  return labels[eventType] || eventType
}

function eventSummary(evt: RunEvent): string {
  const p = evt.payload || {}
  switch (evt.event_type) {
    case 'state_transition':
      return `${p.from_state || '?'} \u2192 ${p.to_state || '?'}`
    case 'memory_loaded':
      return `${p.bundle_count || 0} bundle(s), keys: ${(p.bundle_keys as string[] || []).join(', ') || 'none'}`
    case 'tools_loaded':
      return `${p.tool_count || 0} tool(s): ${(p.tool_names as string[] || []).join(', ') || 'none'}`
    case 'prompt_assembled':
      return `${p.prompt_length || 0} chars, sections: ${(p.memory_sections as string[] || []).join(', ') || 'none'}`
    case 'assistant_delta':
      return `chunk #${p.sequence_no || '?'} (${p.content_length || 0} chars)`
    case 'assistant_final':
      return `${p.content_length || 0} chars, reason: ${p.finish_reason || 'unknown'}`
    case 'tool_started':
      return `${p.tool_name || '?'} \u2014 ${p.args_preview || ''}`
    case 'tool_completed':
      return `${p.tool_name || '?'} (${p.duration_ms || 0}ms) \u2014 ${p.status || '?'}`
    case 'approval_requested':
      return `${p.tool_name || '?'} awaiting approval`
    case 'approval_resolved':
      return `${p.tool_name || '?'} \u2014 ${p.approved ? 'approved' : 'rejected'}`
    case 'run_error':
      return `${p.error_type || 'unknown'}: ${p.error_message || '?'}`
    case 'run_completed':
      return `Response: ${p.response_length || 0} chars`
    default:
      return JSON.stringify(p)
  }
}

function formatEventTime(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 3 })
}
</script>

<style scoped>
.run-timeline {
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  padding: var(--space-4);
}

.timeline-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-4);
}

.timeline-title {
  font-size: var(--text-base);
  font-weight: var(--font-semibold);
}

.timeline-status {
  display: flex;
  align-items: center;
  gap: 6px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
}

.status-dot--live {
  background: var(--color-state-success);
  box-shadow: 0 0 6px var(--color-state-success);
  animation: pulse 1.5s ease-in-out infinite;
}

.status-dot--ended {
  background: var(--color-text-muted);
}

.status-dot--disconnected {
  background: var(--color-state-error);
}

.status-label {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.3px;
  color: var(--color-text-muted);
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

.timeline-empty {
  color: var(--color-text-muted);
  font-size: 13px;
  text-align: center;
  padding: 24px 0;
}

.timeline-list {
  display: flex;
  flex-direction: column;
  gap: 0;
  max-height: 500px;
  overflow-y: auto;
}

.timeline-item {
  display: flex;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid var(--color-border-default);
}

.timeline-item:last-child {
  border-bottom: none;
}

.timeline-marker {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  margin-top: 4px;
  flex-shrink: 0;
}

.timeline-marker--state { background: var(--color-accent-primary); }
.timeline-marker--prepare { background: var(--color-accent-primaryHover); }
.timeline-marker--model { background: var(--color-state-success); }
.timeline-marker--tool { background: var(--color-state-warning); }
.timeline-marker--approval { background: var(--color-state-warning); }
.timeline-marker--error { background: var(--color-state-error); }
.timeline-marker--completed { background: var(--color-state-success); }
.timeline-marker--default { background: var(--color-text-muted); }

.timeline-content {
  flex: 1;
  min-width: 0;
}

.timeline-event-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 2px;
}

.event-type-badge {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 3px;
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

.event-type-badge--state { background: var(--color-accent-primaryMuted); color: var(--color-accent-primary); }
.event-type-badge--prepare { background: var(--color-state-infoMuted); color: var(--color-state-info); }
.event-type-badge--model { background: var(--color-state-successMuted); color: var(--color-state-success); }
.event-type-badge--tool { background: var(--color-state-warningMuted); color: var(--color-state-warning); }
.event-type-badge--approval { background: var(--color-state-warningMuted); color: var(--color-state-warning); }
.event-type-badge--error { background: var(--color-state-errorMuted); color: var(--color-state-error); }
.event-type-badge--completed { background: var(--color-state-successMuted); color: var(--color-state-success); }
.event-type-badge--default { background: var(--color-state-neutralMuted); color: var(--color-text-muted); }

.event-time {
  font-size: 10px;
  color: var(--color-text-muted);
  margin-left: auto;
  font-family: monospace;
}

.event-detail {
  font-size: 12px;
  color: var(--color-text-muted);
  line-height: 1.4;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
