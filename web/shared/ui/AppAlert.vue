<template>
  <div class="app-alert" :class="[`app-alert--${tone}`]">
    <slot />
  </div>
</template>

<script setup lang="ts">
defineProps<{ tone?: 'info' | 'warning' | 'error' | 'success' }>()
</script>

<style scoped>
.app-alert {
  border-radius: var(--radius-sm);
  border: 1px solid var(--color-border-default);
  padding: var(--space-3);
}
.app-alert--info { border-color: var(--color-state-info); }
.app-alert--warning { border-color: var(--color-state-warning); }
.app-alert--error { border-color: var(--color-state-error); }
.app-alert--success { border-color: var(--color-state-success); }
</style>
