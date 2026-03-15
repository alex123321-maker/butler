<template>
  <div class="page">
    <h2 class="page-title">Dashboard</h2>
    <div class="card-grid">
      <div class="card">
        <h3>System Status</h3>
        <SystemStatus :detailed="true" />
      </div>
      <div class="card">
        <h3>Recent Sessions</h3>
        <p class="placeholder-text">Session activity will appear here.</p>
      </div>
      <div class="card">
        <h3>Recent Runs</h3>
        <p class="placeholder-text">Run history will appear here.</p>
      </div>
      <div class="card">
        <h3>Memory</h3>
        <p class="placeholder-text">Memory statistics will appear here.</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
useHead({ title: 'Dashboard — Butler' })
</script>
