<template>
  <div class="page">
    <div class="page-header">
      <NuxtLink to="/sessions" class="back-link">&larr; Sessions</NuxtLink>
      <h2 class="page-title">Session: {{ sessionKey }}</h2>
    </div>

    <div v-if="pending" class="placeholder-text">Loading session...</div>
    <div v-else-if="error" class="placeholder-text">Failed to load session.</div>
    <template v-else-if="data">
      <div class="session-info card">
        <div class="info-row"><span class="info-label">Channel</span><span class="channel-badge">{{ data.session.channel }}</span></div>
        <div class="info-row"><span class="info-label">User</span><span>{{ data.session.user_id }}</span></div>
        <div class="info-row"><span class="info-label">Created</span><span>{{ formatTime(data.session.created_at) }}</span></div>
        <div class="info-row"><span class="info-label">Last Activity</span><span>{{ formatTime(data.session.updated_at) }}</span></div>
      </div>

      <h3 class="section-title">Runs</h3>

      <div v-if="data.runs.length === 0" class="placeholder-text">No runs for this session.</div>
      <table v-else class="data-table">
        <thead>
          <tr>
            <th>Run ID</th>
            <th>State</th>
            <th>Provider</th>
            <th>Started</th>
            <th>Finished</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="run in data.runs" :key="run.run_id || `${run.session_key}-${run.started_at}`">
            <td>
              <NuxtLink v-if="run.run_id" :to="`/runs/${encodeURIComponent(run.run_id)}`" class="run-link">
                {{ shortRunId(run.run_id) }}
              </NuxtLink>
              <span v-else class="placeholder-text">pending</span>
            </td>
            <td>
              <span :class="stateBadgeClass(run.current_state)" class="state-badge">{{ run.current_state }}</span>
            </td>
            <td>{{ run.model_provider }}</td>
            <td>{{ formatTime(run.started_at) }}</td>
            <td>{{ run.finished_at ? formatTime(run.finished_at) : '—' }}</td>
          </tr>
        </tbody>
      </table>
    </template>
  </div>
</template>

<script setup lang="ts">
import { useSessionDetail } from '~/composables/useApi'

const route = useRoute()
const sessionKey = route.params.key as string

useHead({ title: `Session ${sessionKey} — Butler` })

const { data, pending, error } = useSessionDetail(sessionKey)

function formatTime(iso: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString()
}

function shortRunId(id: string): string {
  if (id.length > 24) return id.slice(0, 24) + '...'
  return id
}

function stateBadgeClass(state: string): string {
  if (state === 'completed') return 'state-badge--completed'
  if (state === 'failed' || state === 'timed_out') return 'state-badge--failed'
  if (state === 'model_running' || state === 'tool_running') return 'state-badge--running'
  return 'state-badge--pending'
}
</script>

<style scoped>
.page-header {
  margin-bottom: 24px;
}

.back-link {
  font-size: 13px;
  color: var(--color-text-muted);
  display: inline-block;
  margin-bottom: 8px;
}

.back-link:hover {
  color: var(--color-primary);
}

.session-info {
  margin-bottom: 32px;
}

.info-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 0;
  font-size: 14px;
}

.info-label {
  color: var(--color-text-muted);
  font-weight: 600;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.3px;
  min-width: 120px;
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

.section-title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 16px;
}

.run-link {
  color: var(--color-primary);
  font-family: monospace;
  font-size: 12px;
}
</style>
