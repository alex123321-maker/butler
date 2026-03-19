<template>
  <div v-if="modelValue" class="app-dialog__backdrop" @click.self="$emit('update:modelValue', false)">
    <div class="app-dialog">
      <header class="app-dialog__header">
        <slot name="title">Dialog</slot>
      </header>
      <div class="app-dialog__body">
        <slot />
      </div>
      <footer v-if="$slots.footer" class="app-dialog__footer">
        <slot name="footer" />
      </footer>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ modelValue: boolean }>()
defineEmits<{ (e: 'update:modelValue', value: boolean): void }>()
</script>

<style scoped>
.app-dialog__backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: grid;
  place-items: center;
  z-index: 1000;
}
.app-dialog {
  width: min(560px, 90vw);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
}
.app-dialog__header {
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--color-border-default);
}
.app-dialog__body {
  padding: var(--space-4);
}
.app-dialog__footer {
  padding: var(--space-3) var(--space-4);
  border-top: 1px solid var(--color-border-default);
}
</style>
