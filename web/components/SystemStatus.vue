<template>
  <div class="system-status" :class="statusClass">
    <span class="status-indicator" :class="indicatorClass" />
    <span v-if="detailed" class="status-text">{{ statusText }}</span>
  </div>
</template>

<script setup lang="ts">
const props = withDefaults(defineProps<{ detailed?: boolean }>(), { detailed: false })

const { data, status } = useHealthCheck()

const statusClass = computed(() => ({
  'system-status--healthy': data.value?.healthy === true,
  'system-status--unhealthy': data.value?.healthy === false,
  'system-status--unknown': status.value === 'pending',
}))

const indicatorClass = computed(() => ({
  'status-indicator--green': data.value?.healthy === true,
  'status-indicator--red': data.value?.healthy === false,
  'status-indicator--grey': status.value === 'pending',
}))

const statusText = computed(() => {
  if (status.value === 'pending') return 'Checking...'
  if (data.value?.healthy) return 'System healthy'
  return 'System unhealthy'
})
</script>
