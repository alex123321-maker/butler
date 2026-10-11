<template>
  <div data-testid="guarded-tab-choice">
    <p>Select one browser tab. This permits its binding; it does not execute an action.</p>
    <p v-if="!ownerToken">Enter the owner token to view tab choices.</p>
    <p v-else-if="loading">Loading current tab choices...</p>
    <p v-if="message" role="status">{{ message }}</p>
    <AppButton v-if="ownerToken && !loading" :disabled="busy" @click="load">Refresh tab choices</AppButton>
    <template v-if="choices">
      <p>Expires {{ choices.expires_at }}</p>
      <ul>
        <li v-for="candidate in choices.candidates" :key="candidate.candidate_token">
          <AppButton :disabled="busy || choices.state !== 'pending'" @click="select(candidate.candidate_token)">Tab {{ candidate.ordinal }} · {{ candidate.origin }}</AppButton>
          <span v-if="candidate.active_at_request"> · active when requested</span>
          <span v-if="candidate.selected"> · selected</span>
        </li>
      </ul>
      <AppButton variant="danger" :disabled="busy" @click="revoke">Revoke tab binding</AppButton>
    </template>
  </div>
</template>

<script setup lang="ts">
import AppButton from '~/shared/ui/AppButton.vue'
import { fetchGuardedTabChoices, revokeGuardedTab, selectGuardedTab, type GuardedTabChoices } from '~/entities/approval/api'

const props = defineProps<{ approvalId: string; ownerToken: string }>()
const emit = defineEmits<{ changed: [] }>()
const choices = ref<GuardedTabChoices | null>(null)
const loading = ref(false)
const busy = ref(false)
const message = ref('')
let epoch = 0

const load = async () => {
  const requestEpoch = ++epoch
  choices.value = null
  message.value = ''
  loading.value = Boolean(props.ownerToken)
  busy.value = false
  if (!props.ownerToken) return
  try {
    const current = await fetchGuardedTabChoices(props.approvalId, props.ownerToken)
    if (epoch !== requestEpoch) return
    if (current.approval_id !== props.approvalId) throw new Error('Different tab request')
    choices.value = current
  } catch {
    if (epoch === requestEpoch) message.value = 'Current tab choices are unavailable. No binding was changed.'
  } finally {
    if (epoch === requestEpoch) loading.value = false
  }
}

const decide = async (candidateToken?: string) => {
  if (!props.ownerToken || !choices.value || busy.value) return
  const requestEpoch = epoch
  const id = props.approvalId
  const token = props.ownerToken
  busy.value = true
  message.value = ''
  try {
    if (candidateToken) await selectGuardedTab(id, candidateToken, token)
    else await revokeGuardedTab(id, token)
    if (epoch !== requestEpoch) return
    choices.value = candidateToken && choices.value ? { ...choices.value, state: 'selected', candidates: choices.value.candidates.map(candidate => ({ ...candidate, selected: candidate.candidate_token === candidateToken })) } : null
    message.value = candidateToken ? 'Tab selected. No browser action has been executed.' : 'Tab binding revoked. Previously recorded effects are retained.'
    emit('changed')
  } catch {
    if (epoch === requestEpoch) {
      choices.value = null
      message.value = 'Tab decision could not be confirmed. Refresh the request before acting again.'
    }
  } finally {
    if (epoch === requestEpoch) busy.value = false
  }
}
const select = (candidateToken: string) => decide(candidateToken)
const revoke = () => decide()
watch(() => [props.approvalId, props.ownerToken], load, { immediate: true })
onBeforeUnmount(() => { epoch++; choices.value = null })
</script>
