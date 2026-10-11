<template>
  <main class="page">
    <h2 class="page-title">Approvals</h2>

    <label class="owner-control">Owner control token
      <input v-model="ownerToken" type="password" autocomplete="off" aria-label="Owner control token" />
    </label>
    <p class="placeholder-text">Use the owner token provisioned on your server. It is kept in this page's memory until you leave.</p>

    <AppAlert v-if="hasBlockingError" tone="error">Unable to load approvals.</AppAlert>
    <AppAlert v-else-if="error" tone="warning">{{ error }}</AppAlert>

    <p class="approvals-meta">Total: {{ approvals.length }}<span v-if="pending"> · loading...</span></p>

    <AppTable v-if="approvals.length > 0">
      <thead>
        <tr>
          <th>ID</th>
          <th>Status</th>
          <th>Context</th>
          <th>Run</th>
          <th>Risk</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="approval in approvals" :key="approval.id">
          <td>{{ approval.id }}</td>
          <td>{{ approval.status }}</td>
          <td>{{ approval.summary || `Tool: ${approval.tool_name || '-'}` }}<pre v-if="approval.args_json">{{ approval.args_json }}</pre><small v-if="approval.expires_at">Expires {{ approval.expires_at }} · reply through {{ approval.requested_via }}</small></td>
          <td>{{ approval.run_id }}</td>
          <td>{{ approval.risk_level || '-' }}</td>
          <td>
            <GuardedTabChoice v-if="approval.tool_name === 'single_tab.bind.guarded'" :approval-id="approval.id" :owner-token="ownerToken" @changed="approvalsStore.load()" />
            <div v-else class="approval-actions">
              <AppButton variant="primary" :disabled="approval.status !== 'pending' || pending || actionPending || !ownerToken" @click="approve(approval.id)">Approve</AppButton>
              <AppButton variant="danger" :disabled="approval.status !== 'pending' || pending || actionPending || !ownerToken" @click="rejectApproval(approval.id)">Reject</AppButton>
            </div>
          </td>
        </tr>
      </tbody>
    </AppTable>

    <p v-else-if="pending" class="placeholder-text">Loading approvals...</p>
    <p v-else class="placeholder-text">No pending approvals.</p>
  </main>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import AppAlert from '~/shared/ui/AppAlert.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppTable from '~/shared/ui/AppTable.vue'
import GuardedTabChoice from '~/components/GuardedTabChoice.vue'
import { useApprovalsStore } from '~/shared/model/stores/approvals'

useHead({ title: 'Approvals - Butler' })

const approvalsStore = useApprovalsStore()
const { items: approvals, pending, actionPending, error } = storeToRefs(approvalsStore)
const ownerToken = ref('')
onBeforeUnmount(() => { ownerToken.value = '' })

const hasBlockingError = computed(() => {
  return approvals.value.length === 0 && Boolean(error.value)
})

const approve = async (id: string) => {
  await approvalsStore.approve(id, ownerToken.value)
}

const rejectApproval = async (id: string) => {
  await approvalsStore.reject(id, ownerToken.value)
}

onMounted(async () => {
  await approvalsStore.load()
})
</script>

<style scoped>
.owner-control { display: flex; gap: var(--space-2); align-items: center; margin-bottom: var(--space-2); }
pre { white-space: pre-wrap; overflow-wrap: anywhere; max-width: 32rem; }
.approvals-meta {
  color: var(--color-text-secondary);
  margin-bottom: var(--space-3);
}

.approval-actions {
  display: flex;
  gap: var(--space-2);
}
</style>
