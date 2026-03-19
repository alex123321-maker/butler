<template>
  <button class="app-button" :class="[`app-button--${variant}`]" :disabled="disabled" v-bind="$attrs">
    <slot />
  </button>
</template>

<script setup lang="ts">
defineProps<{
  variant?: 'primary' | 'secondary' | 'danger'
  disabled?: boolean
}>()
</script>

<style scoped>
.app-button {
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  padding: var(--space-2) var(--space-4);
  background: var(--color-bg-surface);
  color: var(--color-text-primary);
  cursor: pointer;
}
.app-button--primary {
  background: var(--color-accent-primary);
  border-color: var(--color-accent-primary);
  color: var(--color-text-inverse);
}
.app-button--danger {
  background: var(--color-state-error);
  border-color: var(--color-state-error);
  color: var(--color-text-inverse);
}
.app-button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
