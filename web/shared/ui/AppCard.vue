<template>
  <section class="app-card">
    <slot />
  </section>
</template>

<style scoped>
.app-card {
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  padding: var(--space-4);
}
</style>
