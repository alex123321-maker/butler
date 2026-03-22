<template>
  <aside class="sidebar" :class="{ 'is-collapsed': isCollapsed }">
    <div class="sidebar-header">
      <div class="brand-block" v-if="!isCollapsed">
        <div class="logo">Butler</div>
        <p class="brand-copy">Personal control panel</p>
      </div>
      <button class="toggle-btn" type="button" @click="toggleSidebar">
        <span aria-hidden="true">{{ isCollapsed ? '+' : '-' }}</span>
        <span class="sr-only">{{ isCollapsed ? 'Expand sidebar' : 'Collapse sidebar' }}</span>
      </button>
    </div>

    <div class="sidebar-section" v-if="!isCollapsed">
      <p class="section-label">Workspace</p>
    </div>
    <nav class="sidebar-nav">
      <NuxtLink v-for="item in workspaceItems" :key="item.path" :to="item.path" class="nav-item" active-class="is-active">
        <span class="icon" aria-hidden="true">{{ item.icon }}</span>
        <span class="label" v-if="!isCollapsed">{{ item.label }}</span>
      </NuxtLink>
    </nav>

    <div class="sidebar-section sidebar-section--advanced">
      <button
        class="advanced-toggle"
        type="button"
        :aria-label="advancedOpen ? 'Hide advanced navigation' : 'Show advanced navigation'"
        @click="advancedOpen = !advancedOpen"
      >
        <span v-if="!isCollapsed">Advanced</span>
        <span aria-hidden="true">{{ advancedOpen ? '-' : '+' }}</span>
      </button>
    </div>

    <nav v-if="advancedOpen" class="sidebar-nav sidebar-nav--advanced">
      <NuxtLink v-for="item in advancedItems" :key="item.path" :to="item.path" class="nav-item nav-item--subtle" active-class="is-active">
        <span class="icon" aria-hidden="true">{{ item.icon }}</span>
        <span class="label" v-if="!isCollapsed">{{ item.label }}</span>
      </NuxtLink>
    </nav>
  </aside>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'

const isCollapsed = ref(false)
const route = useRoute()

const toggleSidebar = () => {
  isCollapsed.value = !isCollapsed.value
}

const workspaceItems = [
  { path: '/', label: 'Home', icon: 'H' },
  { path: '/tasks', label: 'Tasks', icon: 'T' },
  { path: '/approvals', label: 'Approvals', icon: 'A' },
  { path: '/artifacts', label: 'Results', icon: 'R' },
  { path: '/memory', label: 'Memory', icon: 'M' },
  { path: '/settings', label: 'Settings', icon: 'S' },
]

const advancedItems = [
  { path: '/sessions', label: 'Sessions', icon: 'S' },
  { path: '/activity', label: 'Activity', icon: 'A' },
  { path: '/system', label: 'System', icon: 'Y' },
  { path: '/doctor', label: 'Doctor', icon: 'D' },
]

const advancedOpen = ref(false)

const advancedRoutes = computed(() => advancedItems.map((item) => item.path))

watch(
  () => route.path,
  (path) => {
    if (advancedRoutes.value.some((prefix) => path.startsWith(prefix))) {
      advancedOpen.value = true
    }
  },
  { immediate: true },
)
</script>

<style scoped>
.sidebar {
  width: 264px;
  background-color: var(--color-bg-surface);
  border-right: 1px solid var(--color-border-default);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  transition: width 0.2s ease;
}
.sidebar.is-collapsed {
  width: 84px;
}
.sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-4);
  border-bottom: 1px solid var(--color-border-default);
}
.brand-block {
  display: grid;
  gap: var(--space-1);
}
.logo {
  font-weight: bold;
}
.brand-copy {
  color: var(--color-text-secondary);
  font-size: var(--text-sm);
  margin: 0;
}
.is-collapsed .logo {
  display: none;
}
.toggle-btn {
  width: 32px;
  height: 32px;
  border-radius: var(--radius-sm);
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-default);
  color: inherit;
  cursor: pointer;
}
.toggle-btn:hover {
  background: var(--color-bg-surfaceMuted);
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  border: 0;
}
.sidebar-section {
  padding: 0 var(--space-4);
}
.section-label {
  margin: 0;
  font-size: var(--text-xs);
  letter-spacing: var(--tracking-widest);
  text-transform: uppercase;
  color: var(--color-text-muted);
}
.advanced-toggle {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  background: transparent;
  color: var(--color-text-secondary);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  cursor: pointer;
}
.advanced-toggle:hover {
  background: var(--color-bg-surfaceMuted);
}
.sidebar-nav {
  display: flex;
  flex-direction: column;
  padding: 0 var(--space-3);
  gap: var(--space-1);
}
.sidebar-nav--advanced {
  padding-bottom: var(--space-4);
}
.nav-item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3);
  color: var(--color-text-secondary);
  text-decoration: none;
  border-radius: var(--radius-md);
  transition: background-color 0.2s;
}
.nav-item:hover {
  background-color: var(--color-bg-surfaceMuted);
}
.nav-item.is-active {
  color: var(--color-text-primary);
  background-color: var(--color-bg-elevated);
  box-shadow: inset 2px 0 0 var(--color-accent-primary);
}
.nav-item--subtle:not(.is-active) {
  color: var(--color-text-muted);
}
.icon {
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  background: var(--color-bg-surfaceMuted);
  text-align: center;
  font-size: var(--text-xs);
  font-weight: var(--font-semibold);
}
.is-collapsed .icon {
  margin-right: 0;
}
.label {
  font-size: var(--text-base);
  font-weight: var(--font-medium);
}
.is-collapsed .brand-copy,
.is-collapsed .section-label,
.is-collapsed .advanced-toggle span:first-child,
.is-collapsed .label {
  display: none;
}
.is-collapsed .sidebar-nav,
.is-collapsed .sidebar-section {
  padding-left: var(--space-2);
  padding-right: var(--space-2);
}
</style>
