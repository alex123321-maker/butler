<template>
  <AppPanel title="Owner controls">
    <p v-if="record">Task version {{ record.revision }} · {{ record.status }} · {{ record.execution_policy }}</p>
    <p>Changes revoke the previous attempt. Already dispatched effects may need reconciliation.</p>
    <label>Owner control token
      <input v-model="ownerToken" type="password" autocomplete="off" aria-label="Owner control token" />
    </label>
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
const canControl = computed(() => Boolean(ownerToken.value && record.value && !pending.value && !['cancelled', 'completed'].includes(record.value.status)))
onBeforeUnmount(() => { ownerToken.value = '' })
watch(() => props.taskId, async (id) => {
  ownerToken.value = ''
  result.value = null
  try {
    const response = await apiRequest<{ task: ControlTask }>(`/api/v3/tasks/${encodeURIComponent(id)}`)
    record.value = response.task
  } catch { error.value = 'Unable to load current task version.' }
}, { immediate: true })

const apply = async (kind: 'amend' | 'pause' | 'resume' | 'stop') => {
  if (!canControl.value || !record.value) return
  pending.value = true
  error.value = ''
  result.value = null
  try {
    const deliveryId = [...crypto.getRandomValues(new Uint8Array(16))].map((value) => value.toString(16).padStart(2, '0')).join('')
    const applied = await apiRequest<ControlResult>(`/api/v3/tasks/${encodeURIComponent(props.taskId)}/control`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${ownerToken.value}`, 'X-Butler-Delivery-ID': deliveryId },
      body: { kind, expected_revision: record.value.revision, instruction: kind === 'amend' ? instruction.value : '', execution_policy: kind === 'amend' && prepareOnly.value ? 'prepare_only' : undefined },
    })
    result.value = applied
    record.value = applied.task
    emit('changed')
  } catch {
    error.value = 'Control was not applied. Check owner authentication and reload the current task version.'
  } finally { pending.value = false }
}
</script>

<style scoped>
label { display: flex; gap: var(--space-2); margin-block: var(--space-2); }
textarea { flex: 1; }
.control-actions { display: flex; flex-wrap: wrap; gap: var(--space-2); margin-block: var(--space-3); }
</style>
