<template>
  <div class="app-table-wrap">
    <table class="app-table">
      <slot />
    </table>
  </div>
</template>

<style scoped>
.app-table-wrap {
  overflow: auto;
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
}
.app-table {
  width: 100%;
  border-collapse: collapse;
}
.app-table :deep(th),
.app-table :deep(td) {
  border-bottom: 1px solid var(--color-border-default);
  padding: var(--space-3);
  text-align: left;
}
.app-table :deep(th) {
  color: var(--color-text-secondary);
}
</style>
