<template>
  <main class="page">
    <h2 class="page-title">Overview</h2>
    <AppAlert v-if="hasBlockingError" tone="error">Failed to load overview data.</AppAlert>

    <div v-else-if="pending" class="card-grid">
      <AppCard v-for="item in 4" :key="item">
        <p class="placeholder-text">loading...</p>
      </AppCard>
    </div>

    <AppAlert v-else-if="error" tone="warning">Overview loaded partially. Showing available counters.</AppAlert>

    <AppAlert v-if="isEmpty" tone="info">No overview data yet.</AppAlert>

    <div v-else class="card-grid">
      <AppCard>
        <h3>Attention Items</h3>
        <p class="placeholder-text">{{ counts.attention_items_count }}</p>
      </AppCard>
      <AppCard>
        <h3>Active Tasks</h3>
        <p class="placeholder-text">{{ counts.active_tasks_count }}</p>
      </AppCard>
      <AppCard>
        <h3>Approvals Pending</h3>
        <p class="placeholder-text">{{ counts.approvals_pending_count }}</p>
      </AppCard>
      <AppCard>
        <h3>Failed Tasks</h3>
        <p class="placeholder-text">{{ counts.failed_tasks_count }}</p>
      </AppCard>
    </div>
  </main>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import AppAlert from '~/shared/ui/AppAlert.vue'
import AppCard from '~/shared/ui/AppCard.vue'
import { useOverviewStore } from '~/shared/model/stores/overview'

useHead({ title: 'Overview - Butler' })

const overviewStore = useOverviewStore()
const { counts, data, pending, error } = storeToRefs(overviewStore)

const hasBlockingError = computed(() => {
  return !data.value && Boolean(error.value)
})

const isEmpty = computed(() => {
  if (!data.value) {
    return false
  }

  return (
    (counts.value.attention_items_count ?? 0) === 0 &&
    (counts.value.active_tasks_count ?? 0) === 0 &&
    (counts.value.approvals_pending_count ?? 0) === 0 &&
    (counts.value.failed_tasks_count ?? 0) === 0
  )
})

onMounted(async () => {
  await overviewStore.load()
})
</script>
