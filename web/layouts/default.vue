<template>
  <div class="app-layout">
    <WidgetSidebar />
    <div class="content-wrapper">
      <div class="content-shell">
        <slot />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import WidgetSidebar from '~/widgets/sidebar/ui/WidgetSidebar.vue'
</script>

<style scoped>
.app-layout {
  display: flex;
  min-height: 100vh;
  background-color: var(--color-bg-canvas);
  color: var(--color-text-primary);
}
.content-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  padding: var(--space-6) var(--space-8);
}

.content-shell {
  width: 100%;
}

@media (max-width: 900px) {
  .content-wrapper {
    padding: var(--space-4);
  }
}
</style>
