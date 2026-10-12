<template>
  <AppPanel title="Owner controls">
    <p v-if="record">Task version {{ record.revision }} · {{ record.status }} · {{ record.execution_policy }}</p>
    <p>Changes revoke the previous attempt. Already dispatched effects may need reconciliation.</p>
    <label>Owner control token
      <input v-model="ownerToken" type="password" autocomplete="off" aria-label="Owner control token" />
    </label>
    <AppButton :disabled="!ownerToken || observationsPending" @click="loadObservations">Load task observations</AppButton>
    <AppAlert v-if="observationsError" tone="error">Task observations could not be read. Recorded actions and costs are unavailable.</AppAlert>
    <section v-if="observations" class="owner-observations">
      <h3>Recorded task observations</h3>
      <p>Snapshot of task version {{ observations.revision }} at {{ formatDateTime(observations.observed_at) }}.</p>
      <p v-if="observations.omitted_metadata?.length">Some references cannot be shown as safe typed metadata.</p>
      <p>Task {{ observations.status || 'state unavailable' }} · {{ observations.completion_result_id ? `Verified completion result ${observations.completion_result_id}` : 'No verified task completion recorded' }}.</p>
      <section v-if="observations.assignment">
        <h4>Purpose and authority</h4>
        <p>{{ observations.assignment.purpose }} · {{ observations.assignment.initiative_reason }}</p>
        <p>Goal {{ observations.assignment.goal_id }} · mandate {{ observations.assignment.mandate_id || observations.assignment.owner_mandate_id }}</p>
        <p>Assignment pins goal version {{ observations.assignment.goal_revision ?? 'unavailable' }} and mandate version {{ observations.assignment.mandate_revision ?? 'unavailable' }}.</p>
        <p v-if="observations.authority?.available">Current goal version {{ observations.authority.goal_revision }} · {{ observations.authority.goal_status }}; mandate version {{ observations.authority.mandate_revision }} · {{ observations.authority.mandate_status }}. {{ observations.authority.versions_current ? 'Stored authority versions match.' : 'Stored authority is stale or inactive.' }} Admission still checks current policy and sources.</p>
        <p v-else>Current authority metadata is unavailable.</p>
        <p v-if="observations.assignment.observation_ref">Observation reference {{ observations.assignment.observation_ref }}</p>
        <NuxtLink to="/goals">Inspect or revoke owner authority</NuxtLink>
      </section>
      <section v-if="observations.initiative">
        <h4>Initiative</h4>
        <p>{{ observations.initiative.status }} · {{ observations.initiative.reason }}</p>
        <p v-if="observations.initiative.source_id">Source {{ observations.initiative.source_id }} · observation {{ observations.initiative.observation_ref || 'unavailable' }}</p>
        <p>Importance {{ observations.initiative.importance }} · confidence score {{ observations.initiative.confidence }} · estimated model calls {{ observations.initiative.expected_model_calls }}.</p>
        <NuxtLink to="/initiatives">Inspect the initiative source</NuxtLink>
      </section>
      <section>
        <h4>Internal child tasks</h4>
        <p>Butler collects their results within this task. Separate conversations are optional.</p>
        <ul v-if="observations.children.length">
          <li v-for="child in observations.children" :key="child.task_id">
            <NuxtLink :to="`/tasks/${encodeURIComponent(child.task_id)}`">{{ child.task_id }}</NuxtLink>
            · {{ child.status }}<span v-if="child.delegation_state"> · delegation {{ child.delegation_state }}</span>
            · {{ child.completion_result_id ? 'Verified task result recorded' : 'Task result is unverified' }}
            <AppButton variant="danger" :disabled="!canStopBranch || ['cancelled', 'completed'].includes(child.status)" @click="apply('stop', { taskId: child.task_id, revision: child.revision })">Stop branch {{ child.task_id }}</AppButton>
          </li>
        </ul>
        <p v-else>No child tasks are recorded.</p>
        <p v-if="observations.children_truncated">Only the first 100 child tasks are shown.</p>
      </section>
      <section>
        <h4>Saved attempts and result references</h4>
        <p>Model response, task verification and notification delivery are separate records.</p>
        <ul v-if="observations.attempts?.length"><li v-for="attempt in observations.attempts" :key="attempt.run_id">
          <NuxtLink :to="`/runs/${encodeURIComponent(attempt.run_id)}`">{{ attempt.run_id }}</NuxtLink> · task version {{ attempt.task_revision }} · {{ attempt.state }} · response {{ attempt.privacy_redacted ? 'removed by retention policy' : attempt.answer_state }} · notification {{ attempt.delivery_state }}
          <NuxtLink v-if="attempt.artifact_id && !attempt.privacy_redacted" :to="`/artifacts?artifact_id=${encodeURIComponent(attempt.artifact_id)}`">Saved response artifact {{ attempt.artifact_id }} (verification separate)</NuxtLink>
        </li></ul>
        <p v-else>No saved attempt observations are available.</p>
        <ul v-if="observations.results?.length"><li v-for="saved in observations.results" :key="saved.result_id">
          Result {{ saved.result_id }} · {{ saved.verdict }} · task version {{ saved.task_revision }} {{ saved.current_revision ? '(current version)' : '(obsolete version)' }}
          <ul><li v-for="ref in saved.evidence_refs" :key="ref.kind + ref.id"><NuxtLink v-if="evidenceLink(ref.kind, ref.id)" :to="evidenceLink(ref.kind, ref.id)">{{ ref.kind }} {{ ref.id }}</NuxtLink><span v-else>{{ ref.kind }} {{ ref.id }}</span></li></ul>
          <p v-if="saved.omitted_reference_count">{{ saved.omitted_reference_count }} references cannot be shown as safe typed metadata.</p>
        </li></ul>
        <p v-if="observations.attempts_truncated || observations.results_truncated">Only the latest 20 attempts and results are shown.</p>
      </section>
      <section>
        <h4>Action consequences and effects</h4>
        <p>Consequences come from the recorded action contract. Execution status does not measure risk.</p>
        <ul v-if="observations.actions.length">
          <li v-for="action in observations.actions" :key="action.action_id">
            {{ action.tool_name }} · task version {{ action.task_revision }} · {{ effectLabel(action.effect) }} · {{ effectStateLabel(action.effect_state) }}
            <details><summary>Recorded evidence and contract</summary><p>Action {{ action.action_id }} · contract {{ action.contract_hash }}</p><p v-for="ref in action.source_refs" :key="ref.source_id + ref.revision"><NuxtLink :to="`/memory?source_id=${encodeURIComponent(ref.source_id)}`">Source {{ ref.source_id }}</NuxtLink> · pinned revision {{ ref.revision }} · access version {{ ref.access_revision }} · SHA-256 {{ ref.content_hash }}</p></details>
          </li>
        </ul>
        <p v-else>No action intents are recorded for this task.</p>
        <p v-if="observations.actions_truncated">Only the latest 50 actions are shown.</p>
      </section>
      <section>
        <h4>Shared task budget</h4>
        <p>Root task {{ observations.budget.root_task_id }}. {{ observations.budget.model_attempts }} model attempts are recorded across its branches.</p>
        <table v-if="observations.budget.limits && observations.budget.reserved">
          <thead><tr><th>Resource</th><th>Reserved</th><th>Limit</th></tr></thead>
          <tbody><tr v-for="dimension in budgetDimensions" :key="dimension.key"><td>{{ dimension.label }}</td><td>{{ observations.budget.reserved[dimension.key] }}</td><td>{{ observations.budget.limits[dimension.key] }}</td></tr></tbody>
        </table>
        <p v-else>Budget limits and reservations are unavailable.</p>
        <p>Reservations are conservative allowances, not measured consumption or an invoice.</p>
        <p>Observed token subtotal: input {{ observations.budget.known_input_tokens }}, output {{ observations.budget.known_output_tokens }}.</p>
        <p v-if="observations.budget.unknown_input_attempts || observations.budget.unknown_output_attempts">Usage is incomplete: input missing for {{ observations.budget.unknown_input_attempts }} attempts; output missing for {{ observations.budget.unknown_output_attempts }} attempts.</p>
      </section>
      <section>
        <h4>Background work across task branches</h4>
        <p>Recorded queue states do not prove model readiness or permission to run. A finished background job does not verify the task result.</p>
        <ul v-if="observations.auxiliary_work?.length">
          <li v-for="job in observations.auxiliary_work" :key="job.job_id">
            {{ job.kind }} · {{ job.status }} · {{ job.attempts }} attempts
            <span v-if="job.requested_model"> · requested model {{ job.requested_model }}</span>
            · task version {{ job.task_revision }}<span v-if="!job.current_task_version"> (obsolete version)</span>
            <span v-if="job.deadline_elapsed"> · deadline elapsed</span>
          </li>
        </ul>
        <p v-else-if="observations.auxiliary_work">No background jobs are recorded.</p>
        <p v-else>Background queue observations are unavailable.</p>
        <p v-if="observations.auxiliary_work_truncated">Only the latest 50 background jobs are shown.</p>
      </section>
    </section>
    <TaskWorkspaceRecovery :task-id="taskId" :owner-token="ownerToken" />
    <label>Amendment
      <textarea v-model="instruction" aria-label="Amendment" rows="3" />
    </label>
    <label><input v-model="prepareOnly" type="checkbox" /> Only prepare; deny external changes</label>
    <div class="control-actions">
      <AppButton :disabled="!canControl || !instruction.trim()" @click="apply('amend')">Apply amendment</AppButton>
      <AppButton :disabled="!canControl" @click="apply('pause')">Pause task</AppButton>
      <AppButton :disabled="!canControl || !['paused', 'blocked', 'waiting_for_user'].includes(record?.status || '')" @click="apply('resume')">Resume task</AppButton>
      <AppButton variant="danger" :disabled="!canControl" @click="apply('stop')">Stop task</AppButton>
    </div>
    <AppAlert v-if="error" tone="error">{{ error }}</AppAlert>
    <div v-if="unresolvedControl" class="control-actions"><AppButton :disabled="pending || !ownerToken" @click="retryControl">Retry same control</AppButton><AppButton :disabled="pending || !ownerToken" @click="reloadForNewControl">Reload task for a new control</AppButton></div>
    <AppAlert v-if="result" tone="info">
      {{ result.effect_boundary.includes('reconciliation') ? 'Previous dispatch may already have committed. Reconciliation is required.' : 'Future actions from the previous attempt were revoked. Previous effects remain in history.' }}
      <NuxtLink :to="`/inputs/${result.input_id}`">View control receipt</NuxtLink>
    </AppAlert>
  </AppPanel>
</template>

<script setup lang="ts">
import AppPanel from '~/shared/ui/AppPanel.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppAlert from '~/shared/ui/AppAlert.vue'
import TaskWorkspaceRecovery from '~/components/TaskWorkspaceRecovery.vue'
import { apiRequest } from '~/shared/api/client'
import type { TaskOwnerOverview } from '~/entities/task/api'
import { formatDateTime } from '~/entities/task/presentation'

interface ControlTask { revision: number; status: string; execution_policy: string }
interface ControlResult { task: ControlTask; input_id: string; effect_boundary: string }
interface SavedControl { taskId: string; deliveryId: string; body: { kind: string; expected_revision: number; instruction: string; execution_policy?: string } }
const props = defineProps<{ taskId: string }>()
const emit = defineEmits<{ changed: [] }>()
const record = ref<ControlTask | null>(null)
const ownerToken = ref('')
const instruction = ref('')
const prepareOnly = ref(false)
const pending = ref(false)
const error = ref('')
const result = ref<ControlResult | null>(null)
const unresolvedControl = ref<SavedControl | null>(null)
let controlEpoch = 0
const observations = ref<TaskOwnerOverview | null>(null)
const observationsPending = ref(false)
const observationsError = ref(false)
let observationsEpoch = 0
const budgetDimensions = [
  { key: 'model_turns', label: 'Model turns' }, { key: 'tokens', label: 'Token allowance' },
  { key: 'external_calls', label: 'External calls' }, { key: 'children', label: 'Child admissions' },
  { key: 'active_millis', label: 'Active milliseconds' }, { key: 'cost_micros', label: 'Cost allowance in micros' },
  { key: 'egress_bytes', label: 'Outgoing bytes' }, { key: 'provider_input_bytes', label: 'Provider input bytes' },
]
const effectLabels: Record<string, string> = { read: 'Read', prepare: 'Preparation', commit: 'External change', unknown: 'Consequences unknown' }
const effectStateLabels: Record<string, string> = { effect_confirmed: 'Effect confirmed', effect_absent: 'Effect confirmed absent', effect_unknown: 'Effect unknown; reconcile before repeating', prepared: 'Prepared', allowed: 'Authorized', executing: 'Dispatch started; effect unverified' }
const effectLabel = (value: string): string => effectLabels[value] || 'Consequences unknown'
const effectStateLabel = (value: string): string => effectStateLabels[value] || 'Effect evidence unavailable'
const evidenceLink = (kind: string, id: string): string => kind === 'source' ? `/memory?source_id=${encodeURIComponent(id)}` : kind === 'artifact' ? `/artifacts?artifact_id=${encodeURIComponent(id)}` : ''
const clearObservations = () => { observationsEpoch++; observations.value = null; observationsPending.value = false; observationsError.value = false }
watch(ownerToken, () => { controlEpoch++; pending.value = false; unresolvedControl.value = null; result.value = null; error.value = ''; clearObservations() })
const canControl = computed(() => Boolean(ownerToken.value && record.value && !pending.value && !unresolvedControl.value && !['cancelled', 'completed'].includes(record.value.status)))
const canStopBranch = computed(() => Boolean(ownerToken.value && record.value && !pending.value && !unresolvedControl.value))
let taskEpoch = 0
onBeforeUnmount(() => { taskEpoch++; ownerToken.value = ''; instruction.value = '' })
watch(() => props.taskId, async (id) => {
  const epoch = ++taskEpoch
  record.value = null
  ownerToken.value = ''
  instruction.value = ''
  prepareOnly.value = false
  pending.value = false
  error.value = ''
  result.value = null
  unresolvedControl.value = null
  controlEpoch++
  clearObservations()
  try {
    const response = await apiRequest<{ task: ControlTask }>(`/api/v3/tasks/${encodeURIComponent(id)}`)
    if (epoch !== taskEpoch) return
    record.value = response.task
  } catch { if (epoch === taskEpoch) error.value = 'Unable to load current task version.' }
}, { immediate: true })

const loadObservations = async () => {
  if (!ownerToken.value || observationsPending.value) return
  const epoch = ++observationsEpoch
  const task = taskEpoch
  const token = ownerToken.value
  observationsPending.value = true
  observations.value = null
  observationsError.value = false
  try {
    const response = await apiRequest<TaskOwnerOverview>(`/api/v3/tasks/${encodeURIComponent(props.taskId)}/observations`, { headers: { Authorization: `Bearer ${token}` } })
    if (task === taskEpoch && epoch === observationsEpoch) observations.value = response
  } catch { if (task === taskEpoch && epoch === observationsEpoch) observationsError.value = true }
  finally { if (task === taskEpoch && epoch === observationsEpoch) observationsPending.value = false }
}

const apply = async (kind: 'amend' | 'pause' | 'resume' | 'stop', target?: { taskId: string; revision: number }) => {
  if (!(target ? canStopBranch.value : canControl.value) || !record.value) return
  const request: SavedControl = {
    taskId: target?.taskId || props.taskId,
    deliveryId: [...crypto.getRandomValues(new Uint8Array(16))].map(value => value.toString(16).padStart(2, '0')).join(''),
    body: { kind, expected_revision: target?.revision || record.value.revision, instruction: kind === 'amend' ? instruction.value : '', execution_policy: kind === 'amend' && prepareOnly.value ? 'prepare_only' : undefined },
  }
  unresolvedControl.value = request
  await sendControl(request)
}

const sendControl = async (request: SavedControl) => {
  if (!ownerToken.value || pending.value) return
  const epoch = taskEpoch
  const proofEpoch = controlEpoch
  const token = ownerToken.value
  pending.value = true
  error.value = ''
  result.value = null
  clearObservations()
  try {
    const applied = await apiRequest<ControlResult>(`/api/v3/tasks/${encodeURIComponent(request.taskId)}/control`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}`, 'X-Butler-Delivery-ID': request.deliveryId },
      body: request.body,
    })
    if (epoch !== taskEpoch || proofEpoch !== controlEpoch) return
    unresolvedControl.value = null
    result.value = applied
    if (request.taskId === props.taskId) record.value = applied.task
    emit('changed')
  } catch {
    if (epoch === taskEpoch && proofEpoch === controlEpoch) error.value = 'Control outcome is unavailable. Retry the same control to recover its saved receipt, or reload current task state before a new control.'
  } finally { if (epoch === taskEpoch && proofEpoch === controlEpoch) pending.value = false }
}

const retryControl = async () => { if (unresolvedControl.value) await sendControl(unresolvedControl.value) }
const reloadForNewControl = async () => {
  if (!ownerToken.value || pending.value) return
  const epoch = taskEpoch
  const proofEpoch = controlEpoch
  pending.value = true
  try {
    const view = await apiRequest<TaskOwnerOverview>(`/api/v3/tasks/${encodeURIComponent(props.taskId)}/observations`, { headers: { Authorization: `Bearer ${ownerToken.value}` } })
    if (epoch !== taskEpoch || proofEpoch !== controlEpoch) return
    if (!view.status || !view.execution_policy) throw new Error('Current task state unavailable')
    record.value = { revision: view.revision, status: view.status, execution_policy: view.execution_policy }
    unresolvedControl.value = null
    error.value = ''
    observations.value = view
  } catch { if (epoch === taskEpoch && proofEpoch === controlEpoch) error.value = 'Current task state is unavailable. The previous control outcome remains unresolved.' }
  finally { if (epoch === taskEpoch && proofEpoch === controlEpoch) pending.value = false }
}
</script>

<style scoped>
label { display: flex; gap: var(--space-2); margin-block: var(--space-2); }
textarea { flex: 1; }
.control-actions { display: flex; flex-wrap: wrap; gap: var(--space-2); margin-block: var(--space-3); }
</style>
