<template>
  <span class="app-badge" :class="[`app-badge--${tone}`]">
    <slot />
  </span>
</template>

<script setup lang="ts">
defineProps<{
  tone?: 'default' | 'success' | 'warning' | 'error' | 'info'
}>()
</script>

<style scoped>
.app-badge {
  display: inline-flex;
  align-items: center;
  border-radius: var(--radius-sm);
  padding: 2px var(--space-2);
  background: var(--color-bg-elevated);
  color: var(--color-text-primary);
  font-size: 12px;
}
.app-badge--success { color: var(--color-state-success); }
.app-badge--warning { color: var(--color-state-warning); }
.app-badge--error { color: var(--color-state-error); }
.app-badge--info { color: var(--color-state-info); }
</style>
