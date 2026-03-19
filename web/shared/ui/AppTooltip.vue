<template>
  <span class="app-tooltip" :title="text">
    <slot />
  </span>
</template>

<script setup lang="ts">
defineProps<{ text: string }>()
</script>

<style scoped>
.app-tooltip {
  cursor: help;
}
</style>
