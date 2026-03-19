<template>
  <main class="page">
    <h2 class="page-title">Task {{ taskId }}</h2>

    <AppAlert v-if="loadError" tone="error">Unable to load task details.</AppAlert>

    <template v-else-if="pendingBase">
      <AppPanel title="Summary">
        <div class="summary-grid">
          <div v-for="index in 4" :key="index" class="summary-item">
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
      <AppPanel title="Summary">
        <div class="summary-grid">
          <div class="summary-item">
            <p class="summary-label">Status</p>
            <AppBadge :tone="statusTone(taskDetails.summary_bar.status)">{{ taskDetails.summary_bar.status }}</AppBadge>
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
            <p class="summary-label">Source channel</p>
            <p class="summary-value">{{ taskDetails.summary_bar.source_channel || '-' }}</p>
          </div>
        </div>
      </AppPanel>

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
        <AppPanel v-if="activeTab === 'timeline'" title="Timeline">
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
              <p class="timeline-item__meta">{{ event.actor_type }} • {{ formatDate(event.created_at) }}</p>
            </li>
          </ol>

          <div class="artifacts-block">
            <h4>Artifacts</h4>
            <AppAlert v-if="artifactsError" tone="error">Failed to load task artifacts.</AppAlert>
            <AppSkeleton v-else-if="artifactsPending" height="56px" />
            <AppEmptyState
              v-else-if="!artifacts.length"
              title="No artifacts"
              description="No artifacts are linked to this task yet."
            />
            <ul v-else class="artifact-list">
              <li v-for="artifact in artifacts" :key="artifact.artifact_id" class="artifact-item">
                <div>
                  <p class="artifact-title">{{ artifact.title || artifact.artifact_type }}</p>
                  <p class="artifact-summary">{{ artifact.summary || 'No summary' }}</p>
                </div>
                <p class="artifact-meta">{{ artifact.artifact_type }} • {{ formatDate(artifact.created_at) }}</p>
              </li>
            </ul>
          </div>
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
                <span class="conversation-item__time">{{ formatDate(message.created_at) }}</span>
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

const activeTab = ref('timeline')

const isDebugMode = computed(() => {
  const mode = String(route.query.mode || '').toLowerCase()
  const debug = String(route.query.debug || '')
  const operator = String(route.query.operator || '')
  const autonomyMode = taskDetails.value?.task.autonomy_mode || ''

  return mode === 'operator' || mode === 'debug' || debug === '1' || operator === '1' || autonomyMode === 'mode_0'
})

const tabs = computed(() => {
  const base = [
    { label: 'Timeline', value: 'timeline' },
    { label: 'Conversation', value: 'conversation' },
  ]
  if (isDebugMode.value) {
    base.push({ label: 'Debug', value: 'debug' })
  }
  return base
})

const formatDate = (value: string | null | undefined): string => {
  if (!value) {
    return '-'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return date.toLocaleString()
}

const formatTiming = (startedAt: string, finishedAt?: string | null): string => {
  return `${formatDate(startedAt)} -> ${finishedAt ? formatDate(finishedAt) : 'in progress'}`
}

const statusTone = (status: string): 'default' | 'success' | 'warning' | 'error' | 'info' => {
  if (status.includes('error') || status.includes('failed')) {
    return 'error'
  }
  if (status.includes('waiting') || status.includes('approval')) {
    return 'warning'
  }
  if (status.includes('completed')) {
    return 'success'
  }
  return 'info'
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

const loadBase = async () => {
  if (!taskId.value) {
    taskDetails.value = null
    return
  }

  pendingBase.value = true
  loadError.value = null

  try {
    taskDetails.value = await fetchTaskById(taskId.value)
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : 'Failed to load task details'
    taskDetails.value = null
  } finally {
    pendingBase.value = false
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
    const response = await fetchTaskTranscript(taskId.value)
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

watch(
  tabs,
  (value) => {
    if (!value.some((tab) => tab.value === activeTab.value)) {
      activeTab.value = 'timeline'
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
    // Reload detail when operator returns from task list with changed filtering context.
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
  grid-template-columns: repeat(4, minmax(0, 1fr));
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
  font-size: 12px;
}

.summary-value {
  margin: 0;
}

.task-section {
  margin-top: var(--space-4);
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
  font-size: 12px;
}

.artifacts-block {
  margin-top: var(--space-5);
}

.artifact-title {
  margin: 0;
  font-weight: 600;
}

.task-json {
  margin: 0;
  white-space: pre-wrap;
  color: var(--color-text-secondary);
  font-family: 'IBM Plex Mono', monospace;
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
