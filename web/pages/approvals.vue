<template>
  <main class="page">
    <h2 class="page-title">Approvals</h2>

    <AppAlert v-if="hasBlockingError" tone="error">Unable to load approvals.</AppAlert>
    <AppAlert v-else-if="error" tone="warning">Approvals list may be stale. Try refresh.</AppAlert>

    <p class="approvals-meta">Total: {{ approvals.length }}<span v-if="pending"> · loading...</span></p>

    <AppTable v-if="!pending && approvals.length > 0">
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
          <td>{{ approval.summary || `Tool: ${approval.tool_name || '-'}` }}</td>
          <td>{{ approval.run_id }}</td>
          <td>{{ approval.risk_level || '-' }}</td>
          <td>
            <div class="approval-actions">
              <AppButton variant="primary" :disabled="approval.status !== 'pending' || actionPending" @click="approve(approval.id)">Approve</AppButton>
              <AppButton variant="danger" :disabled="approval.status !== 'pending' || actionPending" @click="rejectApproval(approval.id)">Reject</AppButton>
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
import { useApprovalsStore } from '~/shared/model/stores/approvals'

useHead({ title: 'Approvals - Butler' })

const approvalsStore = useApprovalsStore()
const { items: approvals, pending, actionPending, error } = storeToRefs(approvalsStore)

const hasBlockingError = computed(() => {
  return approvals.value.length === 0 && Boolean(error.value)
})

const approve = async (id: string) => {
  await approvalsStore.approve(id)
}

const rejectApproval = async (id: string) => {
  await approvalsStore.reject(id)
}

onMounted(async () => {
  await approvalsStore.load()
})
</script>

<style scoped>
.approvals-meta {
  color: var(--color-text-secondary);
  margin-bottom: var(--space-3);
}

.approval-actions {
  display: flex;
  gap: var(--space-2);
}
</style>
