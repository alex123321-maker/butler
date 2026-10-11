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
      <section v-if="observations.assignment">
        <h4>Purpose and authority</h4>
        <p>{{ observations.assignment.purpose }} · {{ observations.assignment.initiative_reason }}</p>
        <p>Goal {{ observations.assignment.goal_id }} · mandate {{ observations.assignment.mandate_id || observations.assignment.owner_mandate_id }}</p>
        <NuxtLink to="/goals">Inspect or revoke owner authority</NuxtLink>
      </section>
      <section v-if="observations.initiative">
        <h4>Initiative</h4>
        <p>{{ observations.initiative.status }} · {{ observations.initiative.reason }}</p>
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
          </li>
        </ul>
        <p v-else>No child tasks are recorded.</p>
        <p v-if="observations.children_truncated">Only the first 100 child tasks are shown.</p>
      </section>
      <section>
        <h4>Action consequences and effects</h4>
        <p>Consequences come from the recorded action contract. Execution status does not measure risk.</p>
        <ul v-if="observations.actions.length">
          <li v-for="action in observations.actions" :key="action.action_id">
            {{ action.tool_name }} · task version {{ action.task_revision }} · {{ effectLabel(action.effect) }} · {{ effectStateLabel(action.effect_state) }}
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
    </section>
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
import { apiRequest } from '~/shared/api/client'
import type { TaskOwnerOverview } from '~/entities/task/api'
import { formatDateTime } from '~/entities/task/presentation'

interface ControlTask { revision: number; status: string; execution_policy: string }
interface ControlResult { task: ControlTask; input_id: string; effect_boundary: string }
const props = defineProps<{ taskId: string }>()
const emit = defineEmits<{ changed: [] }>()
const record = ref<ControlTask | null>(null)
const ownerToken = ref('')
const instruction = ref('')
const prepareOnly = ref(false)
const pending = ref(false)
const error = ref('')
const result = ref<ControlResult | null>(null)
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
const clearObservations = () => { observationsEpoch++; observations.value = null; observationsPending.value = false; observationsError.value = false }
watch(ownerToken, clearObservations)
const canControl = computed(() => Boolean(ownerToken.value && record.value && !pending.value && !['cancelled', 'completed'].includes(record.value.status)))
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

const apply = async (kind: 'amend' | 'pause' | 'resume' | 'stop') => {
  if (!canControl.value || !record.value) return
  const epoch = taskEpoch
  const id = props.taskId
  pending.value = true
  error.value = ''
  result.value = null
  try {
    const deliveryId = [...crypto.getRandomValues(new Uint8Array(16))].map((value) => value.toString(16).padStart(2, '0')).join('')
    const applied = await apiRequest<ControlResult>(`/api/v3/tasks/${encodeURIComponent(id)}/control`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${ownerToken.value}`, 'X-Butler-Delivery-ID': deliveryId },
      body: { kind, expected_revision: record.value.revision, instruction: kind === 'amend' ? instruction.value : '', execution_policy: kind === 'amend' && prepareOnly.value ? 'prepare_only' : undefined },
    })
    if (epoch !== taskEpoch) return
    result.value = applied
    record.value = applied.task
    clearObservations()
    emit('changed')
  } catch {
    if (epoch === taskEpoch) error.value = 'Control was not applied. Check owner authentication and reload the current task version.'
  } finally { if (epoch === taskEpoch) pending.value = false }
}
</script>

<style scoped>
label { display: flex; gap: var(--space-2); margin-block: var(--space-2); }
textarea { flex: 1; }
.control-actions { display: flex; flex-wrap: wrap; gap: var(--space-2); margin-block: var(--space-3); }
</style>
