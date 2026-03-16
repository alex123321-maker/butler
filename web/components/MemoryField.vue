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
  gap: 4px;
}

.memory-field__label {
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.12em;
  color: rgba(255,255,255,0.45);
}

.memory-field__value {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
}
</style>
