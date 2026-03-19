<template>
  <button class="app-icon-button" v-bind="$attrs">
    <slot />
  </button>
</template>

<style scoped>
.app-icon-button {
  width: 32px;
  height: 32px;
  display: inline-grid;
  place-items: center;
  border-radius: var(--radius-sm);
  border: 1px solid var(--color-border-default);
  background: var(--color-bg-surface);
  color: var(--color-text-primary);
  cursor: pointer;
}
</style>
