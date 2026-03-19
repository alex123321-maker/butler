import { defineStore } from 'pinia'
import { approveById, fetchApprovals, rejectById, type ApprovalItem } from '~/entities/approval/api'

export const useApprovalsStore = defineStore('approvals', () => {
  const items = ref<ApprovalItem[]>([])
  const pending = ref(false)
  const actionPending = ref(false)
  const error = ref<string | null>(null)

  const load = async () => {
    pending.value = true
    error.value = null

    try {
      const response = await fetchApprovals()
      items.value = response.items ?? []
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load approvals'
    } finally {
      pending.value = false
    }
  }

  const approve = async (id: string) => {
    actionPending.value = true
    error.value = null

    try {
      await approveById(id)
      await load()
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to approve request'
    } finally {
      actionPending.value = false
    }
  }

  const reject = async (id: string) => {
    actionPending.value = true
    error.value = null

    try {
      await rejectById(id)
      await load()
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to reject request'
    } finally {
      actionPending.value = false
    }
  }

  return {
    items,
    pending,
    actionPending,
    error,
    load,
    approve,
    reject,
  }
})
