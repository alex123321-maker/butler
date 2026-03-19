<template>
  <div class="app-tabs">
    <button
      v-for="tab in tabs"
      :key="tab.value"
      class="app-tabs__item"
      :class="{ 'is-active': modelValue === tab.value }"
      @click="$emit('update:modelValue', tab.value)"
    >
      {{ tab.label }}
    </button>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  modelValue: string
  tabs: Array<{ label: string; value: string }>
}>()

defineEmits<{ (e: 'update:modelValue', value: string): void }>()
</script>

<style scoped>
.app-tabs {
  display: flex;
  gap: var(--space-2);
}
.app-tabs__item {
  background: var(--color-bg-surface);
  color: var(--color-text-secondary);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  padding: var(--space-2) var(--space-3);
  cursor: pointer;
}
.app-tabs__item.is-active {
  color: var(--color-text-primary);
  border-color: var(--color-accent-primary);
}
</style>
