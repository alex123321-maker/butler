<template>
  <div class="app-skeleton" :style="{ height }"></div>
</template>

<script setup lang="ts">
defineProps<{ height?: string }>()
</script>

<style scoped>
.app-skeleton {
  width: 100%;
  border-radius: var(--radius-sm);
  background: linear-gradient(
    90deg,
    var(--color-bg-surfaceMuted),
    var(--color-bg-elevated),
    var(--color-bg-surfaceMuted)
  );
  animation: pulse 1.2s infinite linear;
}
@keyframes pulse {
  0% { background-position: 0 0; }
  100% { background-position: 240px 0; }
}
</style>
