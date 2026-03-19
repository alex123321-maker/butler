<template>
  <div class="app-layout">
    <WidgetSidebar />
    <div class="content-wrapper">
      <slot />
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
  overflow: hidden;
}
</style>
