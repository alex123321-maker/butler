<template>
  <aside class="sidebar" :class="{ 'is-collapsed': isCollapsed }">
    <div class="sidebar-header">
      <div class="logo">Butler</div>
      <button class="toggle-btn" @click="toggleSidebar">=</button>
    </div>
    <nav class="sidebar-nav">
      <NuxtLink v-for="item in navItems" :key="item.path" :to="item.path" class="nav-item" active-class="is-active">
        <span class="icon">{{ item.icon }}</span>
        <span class="label" v-if="!isCollapsed">{{ item.label }}</span>
      </NuxtLink>
    </nav>
  </aside>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const isCollapsed = ref(false)

const toggleSidebar = () => {
  isCollapsed.value = !isCollapsed.value
}

const navItems = [
  { path: '/', label: 'Overview', icon: 'O' },
  { path: '/tasks', label: 'Tasks', icon: 'T' },
  { path: '/approvals', label: 'Approvals', icon: 'A' },
  { path: '/artifacts', label: 'Artifacts', icon: 'F' },
  { path: '/memory', label: 'Memory', icon: 'M' },
  { path: '/activity', label: 'Activity', icon: 'C' },
  { path: '/system', label: 'System', icon: 'S' },
  { path: '/settings', label: 'Settings', icon: 'G' },
]
</script>

<style scoped>
.sidebar {
  width: 240px;
  background-color: var(--color-bg-surface);
  border-right: 1px solid var(--color-border-default);
  display: flex;
  flex-direction: column;
  transition: width 0.2s ease;
}
.sidebar.is-collapsed {
  width: 64px;
}
.sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-4);
  border-bottom: 1px solid var(--color-border-default);
}
.logo {
  font-weight: bold;
}
.is-collapsed .logo {
  display: none;
}
.toggle-btn {
  background: none;
  border: none;
  color: inherit;
  cursor: pointer;
}
.sidebar-nav {
  display: flex;
  flex-direction: column;
  padding: var(--space-4) 0;
}
.nav-item {
  display: flex;
  align-items: center;
  padding: var(--space-3) var(--space-4);
  color: var(--color-text-secondary);
  text-decoration: none;
  transition: background-color 0.2s;
}
.nav-item:hover {
  background-color: var(--color-bg-surfaceMuted);
}
.nav-item.is-active {
  color: var(--color-text-primary);
  background-color: var(--color-bg-elevated);
  border-left: 2px solid var(--color-accent-primary);
}
.icon {
  width: 24px;
  text-align: center;
  margin-right: 8px;
}
.is-collapsed .icon {
  margin-right: 0;
}
</style>
