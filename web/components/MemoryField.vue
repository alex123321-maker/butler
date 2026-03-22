<template>
  <div class="memory-field">
    <span class="memory-field__label">{{ label }}</span>
    <pre class="memory-field__value">{{ value || '—' }}</pre>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  value?: string | null
  block?: boolean
}>()
</script>

<style scoped>
.memory-field {
  display: grid;
  gap: var(--space-1);
}

.memory-field__label {
  font-size: var(--text-xs);
  text-transform: uppercase;
  letter-spacing: var(--tracking-wider);
  color: var(--color-text-muted);
}

.memory-field__value {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--font-mono);
  font-size: var(--text-sm);
}
</style>
