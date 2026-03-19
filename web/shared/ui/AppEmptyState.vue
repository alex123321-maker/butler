<template>
  <div class="app-empty-state">
    <h3 class="app-empty-state__title">{{ title }}</h3>
    <p class="app-empty-state__text">{{ description }}</p>
  </div>
</template>

<script setup lang="ts">
defineProps<{ title: string; description: string }>()
</script>

<style scoped>
.app-empty-state {
  border: 1px dashed var(--color-border-default);
  border-radius: var(--radius-md);
  padding: var(--space-6);
  text-align: center;
}
.app-empty-state__title {
  margin-bottom: var(--space-2);
}
.app-empty-state__text {
  color: var(--color-text-secondary);
}
</style>
