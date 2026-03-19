<template>
  <section class="app-panel">
    <header v-if="title" class="app-panel__header">{{ title }}</header>
    <div class="app-panel__body">
      <slot />
    </div>
  </section>
</template>

<script setup lang="ts">
defineProps<{ title?: string }>()
</script>

<style scoped>
.app-panel {
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
}
.app-panel__header {
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--color-border-default);
  color: var(--color-text-secondary);
}
.app-panel__body {
  padding: var(--space-4);
}
</style>
