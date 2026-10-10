<template>
  <main class="page">
    <h2 class="page-title">Accepted input {{ inputId }}</h2>
    <AppAlert v-if="error" tone="error">{{ error }}</AppAlert>
    <template v-if="receipt">
      <p data-testid="receipt-status">{{ receipt.status }}</p>
      <p>Session position {{ receipt.session_sequence }} · checkpoint {{ receipt.checkpoint }}</p>
      <p v-if="receipt.error_code">{{ receipt.error_code }}: {{ receipt.error_message }}</p>
      <p v-if="receipt.status === 'waiting_for_user'"><NuxtLink to="/approvals">Respond to the saved approval</NuxtLink></p>
      <p v-if="receipt.status === 'blocked'">Owner decision or reconciliation is required before this session continues.</p>
      <p v-if="receipt.run_id"><NuxtLink :to="`/tasks/${encodeURIComponent(receipt.task_id || receipt.run_id)}`">View {{ receipt.task_id ? 'task and attempt' : 'conversation attempt' }}</NuxtLink></p>
      <p v-if="receipt.status === 'completed'">This attempt finished. Task completion is verified separately.</p>
      <p>Attempts {{ receipt.attempts }}/{{ receipt.max_attempts }} · deadline {{ receipt.expires_at }}</p>
    </template>
  </main>
</template>

<script setup lang="ts">
import AppAlert from '~/shared/ui/AppAlert.vue'
import { fetchInputReceipt, type InputReceipt } from '~/entities/input/api'
const inputId = String(useRoute().params.id)
const receipt = ref<InputReceipt | null>(null)
const error = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
let active = true
const refresh = async () => {
  try { receipt.value = await fetchInputReceipt(inputId); error.value = '' }
  catch { error.value = 'Unable to read accepted input.' }
  if (active && (!receipt.value || ['queued', 'running', 'waiting_for_user'].includes(receipt.value.status))) timer = setTimeout(refresh, 1000)
}
onMounted(refresh)
onBeforeUnmount(() => { active = false; if (timer) clearTimeout(timer) })
</script>
