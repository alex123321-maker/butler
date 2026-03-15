<template>
  <div class="page">
    <h2 class="page-title">Sessions</h2>

    <div v-if="pending" class="placeholder-text">Loading sessions...</div>
    <div v-else-if="error" class="placeholder-text">Failed to load sessions.</div>
    <div v-else-if="!data || data.length === 0" class="placeholder-text">No sessions found.</div>

    <table v-else class="data-table">
      <thead>
        <tr>
          <th>Session Key</th>
          <th>Channel</th>
          <th>User</th>
          <th>Last Activity</th>
          <th>Created</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="session in data" :key="session.session_key">
          <td>
            <NuxtLink :to="`/sessions/${encodeURIComponent(session.session_key)}`" class="session-link">
              {{ session.session_key }}
            </NuxtLink>
          </td>
          <td>
            <span class="channel-badge">{{ session.channel }}</span>
          </td>
          <td>{{ session.user_id }}</td>
          <td>{{ formatTime(session.updated_at) }}</td>
          <td>{{ formatTime(session.created_at) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { useSessions } from '~/composables/useApi'

useHead({ title: 'Sessions — Butler' })

const { data, pending, error } = useSessions()

function formatTime(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return d.toLocaleString()
}
</script>

<style scoped>
.session-link {
  color: var(--color-primary);
  font-weight: 500;
}

.channel-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.3px;
  background: rgba(79, 140, 255, 0.12);
  color: var(--color-primary);
}
</style>
