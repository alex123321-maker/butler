<template>
  <input v-model="model" class="app-input" v-bind="$attrs" />
</template>

<script setup lang="ts">
const model = defineModel<string | number>()
</script>

<style scoped>
.app-input {
  width: 100%;
  background: var(--color-bg-surfaceMuted);
  color: var(--color-text-primary);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  padding: var(--space-2) var(--space-3);
}
</style>
