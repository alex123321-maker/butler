<template>
  <main class="page">
    <section class="page-header">
      <div class="page-header__content">
        <p class="page-kicker">Task detail</p>
        <h2 class="page-title">{{ formatTaskId(taskId) }}</h2>
        <p class="page-copy">
          Review the request, what Butler is waiting for, and the current result without diving into debug data first.
        </p>
      </div>
      <div v-if="taskDetails" class="page-header__actions">
        <NuxtLink class="text-link detail-link" :to="memoryLink">Open related memory</NuxtLink>
      </div>
    </section>

    <AppAlert v-if="loadError" tone="error">Unable to load task details.</AppAlert>

    <template v-else-if="pendingBase">
      <AppPanel title="Summary">
        <div class="summary-grid">
          <div v-for="index in 5" :key="index" class="summary-item">
            <p class="summary-label">Loading</p>
            <AppSkeleton height="18px" />
          </div>
        </div>
      </AppPanel>
      <div class="task-section">
        <AppSkeleton height="38px" />
      </div>
    </template>

    <AppEmptyState
      v-else-if="!taskDetails"
      title="Task not found"
      description="This task is not available or has been removed."
    />

    <template v-else>
      <AppAlert v-if="taskDetails.task.legacy" tone="warning">Legacy work: historical model responses have not been verified as user results.</AppAlert>
      <AppPanel title="Summary">
        <div class="summary-grid">
          <div class="summary-item">
            <p class="summary-label">Status</p>
            <AppBadge :tone="taskTone(taskDetails.summary_bar.status)">{{ formatTaskStatus(taskDetails.summary_bar.status) }}</AppBadge>
          </div>
          <div class="summary-item">
            <p class="summary-label">Risk level</p>
            <AppBadge :tone="riskTone(taskDetails.summary_bar.risk_level)">{{ taskDetails.summary_bar.risk_level || 'unknown' }}</AppBadge>
          </div>
          <div class="summary-item">
            <p class="summary-label">Timing</p>
            <p class="summary-value">{{ formatTiming(taskDetails.summary_bar.started_at, taskDetails.summary_bar.finished_at) }}</p>
          </div>
          <div class="summary-item">
            <p class="summary-label">Source</p>
            <p class="summary-value">{{ formatChannel(taskDetails.summary_bar.source_channel || taskDetails.source.channel) }}</p>
          </div>
          <div class="summary-item">
            <p class="summary-label">What Butler needs</p>
            <p class="summary-value">{{ describeTaskState(taskDetails.task) }}</p>
          </div>
        </div>
      </AppPanel>

      <p v-if="taskDetails.run_ids" class="overview-copy">Attempts: {{ taskDetails.run_ids.length }}. Latest attempt: {{ taskDetails.task.run_state || 'not started' }}.</p>

      <NuxtLink v-if="taskDetails.durable_task?.parent_task_id" :to="`/tasks/${encodeURIComponent(taskDetails.durable_task.parent_task_id)}`">View the parent task</NuxtLink>

      <TaskOwnerControls v-if="taskDetails.run_ids" :task-id="taskId" @changed="refreshOwnerState" />

      <AppAlert
        v-if="taskDetails.waiting_state.user_action_channel === 'telegram'"
        class="task-section"
        tone="warning"
      >
        Action is available only through Telegram. {{ taskDetails.waiting_state.note }}
      </AppAlert>

      <div class="task-section">
        <AppTabs v-model="activeTab" :tabs="tabs" />
      </div>

      <div class="task-section">
        <AppPanel v-if="activeTab === 'overview'" title="Overview">
          <div class="overview-grid">
            <section class="overview-card">
              <h3>Original request</h3>
              <p class="overview-copy">{{ taskDetails.source.source_message_full || taskDetails.source.source_message_preview || 'No source message available.' }}</p>
            </section>
            <section class="overview-card">
              <h3>Waiting state</h3>
              <p class="overview-copy">{{ taskDetails.waiting_state.note || formatWaitingReason(taskDetails.waiting_state.waiting_reason) }}</p>
              <div class="pill-row">
                <span class="pill">{{ formatWaitingReason(taskDetails.waiting_state.waiting_reason) }}</span>
                <span v-if="taskDetails.waiting_state.user_action_channel" class="pill">
                  {{ formatChannel(taskDetails.waiting_state.user_action_channel) }}
                </span>
              </div>
            </section>
            <section class="overview-card">
              <h3>Completion criteria</h3>
              <ul v-if="taskDetails.durable_task?.completion_criteria?.length">
                <li v-for="criterion in taskDetails.durable_task.completion_criteria" :key="criterion">{{ criterion }}</li>
              </ul>
              <p v-else class="overview-copy">Completion criteria are unavailable for this historical task.</p>
              <p class="overview-copy">A saved model response does not verify these criteria.</p>
            </section>
            <section v-if="taskDetails.run_ids?.length" class="overview-card">
              <h3>Attempts</h3>
              <ul><li v-for="runId in taskDetails.run_ids" :key="runId"><NuxtLink :to="`/runs/${encodeURIComponent(runId)}`">{{ runId }}</NuxtLink></li></ul>
            </section>
            <section class="overview-card">
              <h3>Memory context</h3>
              <template v-if="taskDetails.durable_task">
                <p class="overview-copy">The plan and unfinished actions belong to this task and survive each model response.</p>
                <p class="overview-copy">Plan: {{ taskDetails.durable_task.working.plan.join(' → ') || 'No plan saved' }}</p>
                <p class="overview-copy">Pending actions: {{ taskDetails.durable_task.working.pending_actions.join('; ') || 'None saved' }}</p>
                <p class="overview-copy">Constraints: {{ taskDetails.durable_task.constraints.join('; ') || 'None saved' }}</p>
              </template>
              <p v-else class="overview-copy">Session history is available in the related memory view.</p>
              <NuxtLink class="text-link" :to="memoryLink">Open session memory</NuxtLink>
            </section>
          </div>
        </AppPanel>

        <AppPanel v-else-if="activeTab === 'result'" title="Result">
          <div class="overview-grid">
            <section class="overview-card">
              <h3>Outcome</h3>
              <AppBadge v-if="taskDetails.result.has_result && taskDetails.result.verified !== undefined" :tone="taskDetails.result.verified ? 'success' : 'warning'">
                {{ taskDetails.result.verified ? 'Verified user result' : 'Unverified model response' }}
              </AppBadge>
              <p class="overview-copy">{{ taskDetails.result.outcome_summary || 'No final result has been recorded yet.' }}</p>
            </section>
            <section class="overview-card">
              <h3>Error state</h3>
              <p class="overview-copy">{{ taskDetails.error.error_summary || 'No error is currently attached to this task.' }}</p>
            </section>
          </div>

          <TaskExecutionResult class="task-section" :result="taskDetails.final_result" :delivery="taskDetails.delivery" :run-id="taskDetails.task.run_id" />

          <div class="artifacts-block">
            <h4>Artifacts</h4>
            <AppAlert v-if="artifactsError" tone="error">Failed to load task artifacts.</AppAlert>
            <AppSkeleton v-else-if="artifactsPending" height="56px" />
            <AppEmptyState
              v-else-if="!artifacts.length"
              title="No artifacts yet"
              description="Butler has not attached result artifacts to this task."
            />
            <ul v-else class="artifact-list">
              <li v-for="artifact in artifacts" :key="artifact.artifact_id" class="artifact-item">
                <div>
                  <p class="artifact-title">{{ artifact.title || artifact.artifact_type }}</p>
                  <p class="artifact-summary">{{ artifact.summary || artifact.content_text || 'No summary' }}</p>
                </div>
                <p class="artifact-meta">{{ artifact.artifact_type }} • {{ formatDateTime(artifact.created_at) }}</p>
              </li>
            </ul>
          </div>
        </AppPanel>

        <AppPanel v-else-if="activeTab === 'timeline'" title="Timeline">
          <AppAlert v-if="timelineError" tone="error">Failed to load activity timeline.</AppAlert>
          <AppSkeleton v-else-if="timelinePending" height="80px" />
          <AppEmptyState
            v-else-if="!activityItems.length"
            title="No timeline events"
            description="Activity events for this task are not available yet."
          />
          <ol v-else class="timeline-list">
            <li v-for="event in activityItems" :key="event.activity_id" class="timeline-item">
              <div class="timeline-item__row">
                <strong>{{ event.title || event.activity_type }}</strong>
                <AppBadge :tone="severityTone(event.severity)">{{ event.severity || 'info' }}</AppBadge>
              </div>
              <p class="timeline-item__summary">{{ event.summary || 'No summary' }}</p>
              <p class="timeline-item__meta">{{ event.actor_type }} • {{ formatDateTime(event.created_at) }}</p>
            </li>
          </ol>
        </AppPanel>

        <AppPanel v-else-if="activeTab === 'conversation'" title="Conversation">
          <AppAlert v-if="conversationError" tone="error">Failed to load transcript.</AppAlert>
          <AppSkeleton v-else-if="conversationPending" height="80px" />
          <AppEmptyState
            v-else-if="!conversationMessages.length"
            title="No transcript messages"
            description="Conversation transcript is empty for this task."
          />
          <ul v-else class="conversation-list">
            <li v-for="message in conversationMessages" :key="message.message_id" class="conversation-item">
              <div class="conversation-item__row">
                <AppBadge tone="info">{{ message.role }}</AppBadge>
                <span class="conversation-item__time">{{ formatDateTime(message.created_at) }}</span>
              </div>
              <p class="conversation-item__text">{{ message.content }}</p>
            </li>
          </ul>
        </AppPanel>

        <AppPanel v-else title="Debug">
          <AppAlert v-if="!isDebugMode" tone="warning">Debug view is available only in operator/debug mode.</AppAlert>
          <template v-else>
            <AppAlert v-if="debugError" tone="error">Failed to load debug payload.</AppAlert>
            <AppSkeleton v-else-if="debugPending" height="120px" />
            <AppEmptyState
              v-else-if="!debugData"
              title="No debug payload"
              description="Debug endpoint returned no data for this task."
            />
            <pre v-else class="task-json">{{ JSON.stringify(debugData, null, 2) }}</pre>
          </template>
        </AppPanel>
      </div>
    </template>
  </main>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import AppAlert from '~/shared/ui/AppAlert.vue'
import AppBadge from '~/shared/ui/AppBadge.vue'
import AppEmptyState from '~/shared/ui/AppEmptyState.vue'
import AppPanel from '~/shared/ui/AppPanel.vue'
import AppSkeleton from '~/shared/ui/AppSkeleton.vue'
import AppTabs from '~/shared/ui/AppTabs.vue'
import {
  fetchTaskActivity,
  fetchTaskArtifacts,
  fetchTaskById,
  fetchTaskDebug,
  fetchTaskTranscript,
  type TaskActivityItem,
  type TaskArtifact,
  type TaskDebugResponse,
  type TaskDetailResponse,
  type TranscriptMessage,
} from '~/entities/task/api'
import {
  describeTaskState,
  formatChannel,
  formatDateTime,
  formatTaskId,
  formatTaskStatus,
  formatWaitingReason,
  taskTone,
} from '~/entities/task/presentation'
import { useTasksStore } from '~/shared/model/stores/tasks'

const route = useRoute()
const taskId = computed(() => String(route.params.id || ''))
const tasksStore = useTasksStore()
const { filters } = storeToRefs(tasksStore)

const taskDetails = ref<TaskDetailResponse | null>(null)
const activityItems = ref<TaskActivityItem[]>([])
const artifacts = ref<TaskArtifact[]>([])
const conversationMessages = ref<TranscriptMessage[]>([])
const debugData = ref<TaskDebugResponse | null>(null)

const pendingBase = ref(false)
const timelinePending = ref(false)
const artifactsPending = ref(false)
const conversationPending = ref(false)
const debugPending = ref(false)

const loadError = ref<string | null>(null)
const timelineError = ref<string | null>(null)
const artifactsError = ref<string | null>(null)
const conversationError = ref<string | null>(null)
const debugError = ref<string | null>(null)

const activeTab = ref('overview')

const isDebugMode = computed(() => {
  const mode = String(route.query.mode || '').toLowerCase()
  const debug = String(route.query.debug || '')
  const operator = String(route.query.operator || '')
  const autonomyMode = taskDetails.value?.task.autonomy_mode || ''

  return mode === 'operator' || mode === 'debug' || debug === '1' || operator === '1' || autonomyMode === 'mode_0'
})

const tabs = computed(() => {
  const base = [
    { label: 'Overview', value: 'overview' },
    { label: 'Result', value: 'result' },
    { label: 'Timeline', value: 'timeline' },
    { label: 'Conversation', value: 'conversation' },
  ]
  if (isDebugMode.value) {
    base.push({ label: 'Debug', value: 'debug' })
  }
  return base
})

const memoryLink = computed(() => {
  if (!taskDetails.value?.task.session_key) {
    return '/memory'
  }
  return {
    path: '/memory',
    query: {
      scopeType: 'session',
      scopeId: taskDetails.value.task.session_key,
      sourceTask: taskDetails.value.task.task_id,
    },
  }
})

const formatTiming = (startedAt: string, finishedAt?: string | null): string => {
  return `${formatDateTime(startedAt)} -> ${finishedAt ? formatDateTime(finishedAt) : 'in progress'}`
}

const riskTone = (riskLevel: string): 'default' | 'success' | 'warning' | 'error' | 'info' => {
  if (riskLevel === 'high') {
    return 'error'
  }
  if (riskLevel === 'medium') {
    return 'warning'
  }
  if (riskLevel === 'low') {
    return 'success'
  }
  return 'default'
}

const severityTone = (severity: string): 'default' | 'success' | 'warning' | 'error' | 'info' => {
  if (severity === 'error') {
    return 'error'
  }
  if (severity === 'warning') {
    return 'warning'
  }
  return 'info'
}

const loadBase = async (background = false) => {
  if (!taskId.value) {
    taskDetails.value = null
    return
  }

  if (!background) pendingBase.value = true
  loadError.value = null

  try {
    taskDetails.value = await fetchTaskById(taskId.value)
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : 'Failed to load task details'
    taskDetails.value = null
  } finally {
    if (!background) pendingBase.value = false
  }
}

const loadTimeline = async () => {
  timelinePending.value = true
  timelineError.value = null

  try {
    const response = await fetchTaskActivity(taskId.value)
    activityItems.value = response.activity
  } catch (err) {
    timelineError.value = err instanceof Error ? err.message : 'Failed to load timeline'
    activityItems.value = []
  } finally {
    timelinePending.value = false
  }
}

const loadArtifacts = async () => {
  artifactsPending.value = true
  artifactsError.value = null

  try {
    const response = await fetchTaskArtifacts(taskId.value)
    artifacts.value = response.artifacts
  } catch (err) {
    artifactsError.value = err instanceof Error ? err.message : 'Failed to load artifacts'
    artifacts.value = []
  } finally {
    artifactsPending.value = false
  }
}

const loadConversation = async () => {
  conversationPending.value = true
  conversationError.value = null

  try {
    const runId = taskDetails.value?.task.run_id || taskId.value
    const response = await fetchTaskTranscript(runId)
    conversationMessages.value = response.messages
  } catch (err) {
    conversationError.value = err instanceof Error ? err.message : 'Failed to load conversation'
    conversationMessages.value = []
  } finally {
    conversationPending.value = false
  }
}

const loadDebug = async () => {
  if (!isDebugMode.value || debugData.value) {
    return
  }

  debugPending.value = true
  debugError.value = null

  try {
    debugData.value = await fetchTaskDebug(taskId.value)
  } catch (err) {
    debugError.value = err instanceof Error ? err.message : 'Failed to load debug payload'
    debugData.value = null
  } finally {
    debugPending.value = false
  }
}

const loadAll = async () => {
  debugData.value = null
  await loadBase()

  if (!taskDetails.value) {
    return
  }

  await Promise.all([loadTimeline(), loadConversation(), loadArtifacts()])
}

const refreshOwnerState = async () => {
  debugData.value = null
  await loadBase(true)
  await Promise.all([loadTimeline(), loadConversation(), loadArtifacts()])
}

watch(
  tabs,
  (value) => {
    if (!value.some((tab) => tab.value === activeTab.value)) {
      activeTab.value = 'overview'
    }
  },
  { immediate: true },
)

watch(activeTab, async (value) => {
  if (value === 'debug') {
    await loadDebug()
  }
})

watch(taskId, async () => {
  await loadAll()
})

watch(
  () => filters.value,
  async () => {
    await loadAll()
  },
  { deep: true },
)

onMounted(async () => {
  await loadAll()
})
</script>

<style scoped>
.summary-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: var(--space-4);
}

.summary-item {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.summary-label {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: var(--text-xs);
}

.summary-value {
  margin: 0;
}

.task-section {
  margin-top: var(--space-4);
}

.detail-link {
  align-self: flex-start;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: var(--space-4);
}

.overview-card {
  display: grid;
  gap: var(--space-3);
  padding: var(--space-4);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  background: var(--color-bg-surfaceMuted);
}

.overview-card h3 {
  margin: 0;
}

.overview-copy {
  margin: 0;
  color: var(--color-text-secondary);
}

.timeline-list,
.conversation-list,
.artifact-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.timeline-item,
.conversation-item,
.artifact-item {
  padding: var(--space-3) 0;
  border-bottom: 1px solid var(--color-border-default);
}

.timeline-item__row,
.conversation-item__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}

.timeline-item__summary,
.conversation-item__text,
.artifact-summary {
  margin: var(--space-2) 0 0;
}

.timeline-item__meta,
.conversation-item__time,
.artifact-meta {
  margin: var(--space-2) 0 0;
  color: var(--color-text-secondary);
  font-size: var(--text-xs);
}

.artifacts-block {
  margin-top: var(--space-5);
  display: grid;
  gap: var(--space-3);
}

.artifact-title {
  margin: 0;
  font-weight: 600;
}

.task-json {
  margin: 0;
  white-space: pre-wrap;
  color: var(--color-text-secondary);
  font-family: var(--font-mono);
}

@media (max-width: 900px) {
  .summary-grid {
    grid-template-columns: 1fr 1fr;
  }
}

@media (max-width: 640px) {
  .summary-grid {
    grid-template-columns: 1fr;
  }
}
</style>
