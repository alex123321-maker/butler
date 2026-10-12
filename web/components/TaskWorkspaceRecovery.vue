<template>
  <section class="workspace-recovery">
    <h3>Quarantined workspace copies</h3>
    <p>Discard erases this stopped task's quarantined working copy. The original operation remains unknown. Active tasks, stale sources or an unexpired executor lease deny discard.</p>
    <AppButton :disabled="!ownerToken || pending" @click="load">Load workspace recovery</AppButton>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-if="statuses && !statuses.length">No recovery copies are recorded for this task.</p>
    <ul v-if="statuses"><li v-for="status in statuses" :key="status.workspace_id">
      Workspace {{ status.workspace_id }} · {{ status.phase }} · original outcome {{ status.original_outcome }}
      <p>Task version {{ status.task_revision }} · constraints {{ status.constraints_revision }} · copy generation {{ status.workspace_generation }} · resource version {{ status.resource_version }}</p>
      <p v-if="status.phase === 'requested' || status.phase === 'erasing'">Owner intent is recorded; erasure is not yet independently confirmed.</p>
      <p v-if="status.phase === 'discarded'">Erasure receipt verified. The original operation remains unknown.</p>
      <AppButton variant="danger" :disabled="!status.can_discard || pending || !!saved" @click="discard(status)">{{ status.recovery_id ? 'Request another discard attempt' : 'Discard this quarantined copy' }}</AppButton>
    </li></ul>
    <p v-if="mayHaveMore">This view is bounded to 50 copies; additional history may exist.</p>
    <AppButton v-if="saved" :disabled="pending || !ownerToken" @click="send(saved)">Retry same discard intent</AppButton>
  </section>
</template>

<script setup lang="ts">
import AppButton from '~/shared/ui/AppButton.vue'
import { apiRequest } from '~/shared/api/client'

interface Status { workspace_id: string; task_id: string; task_revision: number; constraints_revision: number; workspace_generation: number; resource_version: number; recovery_id?: string; phase: string; can_discard: boolean; original_outcome: string }
interface Intent { delivery: string; body: { workspace_id: string; resource_version: number; workspace_generation: number; task_revision: number; constraints_revision: number; previous_recovery_id?: string } }
const props = defineProps<{ taskId: string; ownerToken: string }>()
const statuses = ref<Status[] | null>(null)
const mayHaveMore = ref(false)
const pending = ref(false)
const error = ref('')
const saved = ref<Intent | null>(null)
let epoch = 0
const clear = () => { epoch++; statuses.value = null; saved.value = null; pending.value = false; error.value = ''; mayHaveMore.value = false }
watch(() => [props.taskId, props.ownerToken], clear, { flush: 'sync' })
onBeforeUnmount(clear)
const load = async () => {
  if (!props.ownerToken || pending.value) return
  const version = epoch
  const token = props.ownerToken
  pending.value = true
  error.value = ''
  try {
    const response = await apiRequest<{ workspaces: Status[]; may_have_more: boolean }>(`/api/v3/tasks/${encodeURIComponent(props.taskId)}/workspace-recovery`, { headers: { Authorization: `Bearer ${token}` } })
    if (version !== epoch) return
    statuses.value = response.workspaces
    mayHaveMore.value = response.may_have_more
  } catch { if (version === epoch) { statuses.value = null; error.value = 'Workspace recovery is unavailable for the current owner proof.' } }
  finally { if (version === epoch) pending.value = false }
}
const discard = async (status: Status) => {
  if (!status.can_discard || pending.value || saved.value || !props.ownerToken) return
  const intent: Intent = { delivery: crypto.randomUUID(), body: { workspace_id: status.workspace_id, resource_version: status.resource_version, workspace_generation: status.workspace_generation, task_revision: status.task_revision, constraints_revision: status.constraints_revision, previous_recovery_id: status.recovery_id } }
  saved.value = intent
  await send(intent)
}
const send = async (intent: Intent) => {
  if (!props.ownerToken || pending.value) return
  const version = epoch
  const token = props.ownerToken
  pending.value = true
  error.value = ''
  try {
    const current = await apiRequest<Status>(`/api/workspaces/${encodeURIComponent(intent.body.workspace_id)}/discard`, { method: 'POST', headers: { Authorization: `Bearer ${token}`, 'X-Butler-Delivery-ID': intent.delivery }, body: intent.body })
    if (version !== epoch) return
    saved.value = null
    statuses.value = (statuses.value || []).map(status => status.workspace_id === current.workspace_id ? current : status)
  } catch { if (version === epoch) error.value = 'Discard outcome is unavailable. Retry the same intent to recover its receipt; erasure and the original operation remain unconfirmed.' }
  finally { if (version === epoch) pending.value = false }
}
</script>
