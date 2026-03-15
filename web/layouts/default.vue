<template>
  <div class="app-layout">
    <aside class="sidebar">
      <div class="sidebar-header">
        <h1 class="sidebar-title">Butler</h1>
        <SystemStatus />
      </div>
      <nav class="sidebar-nav">
        <NuxtLink to="/" class="nav-link" active-class="nav-link--active">
          <span class="nav-icon">◉</span> Dashboard
        </NuxtLink>
        <NuxtLink to="/sessions" class="nav-link" active-class="nav-link--active">
          <span class="nav-icon">◎</span> Sessions
        </NuxtLink>
        <NuxtLink to="/memory" class="nav-link" active-class="nav-link--active">
          <span class="nav-icon">◈</span> Memory
        </NuxtLink>
        <NuxtLink to="/doctor" class="nav-link" active-class="nav-link--active">
          <span class="nav-icon">◇</span> Doctor
        </NuxtLink>
        <NuxtLink to="/settings" class="nav-link" active-class="nav-link--active">
          <span class="nav-icon">◆</span> Settings
        </NuxtLink>
      </nav>
    </aside>
    <main class="main-content">
      <slot />
    </main>
  </div>
</template>

<script setup lang="ts">
</script>
