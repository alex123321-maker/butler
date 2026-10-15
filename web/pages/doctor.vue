<template>
  <div class="page">
    <div class="page-header">
      <h2 class="page-title">Doctor</h2>
	  <button class="check-btn" :disabled="checking || !ownerToken" @click="triggerCheck">
        {{ checking ? 'Running...' : 'Run System Check' }}
      </button>
    </div>
	<div class="owner-access"><label class="owner-control">Owner control token <input v-model="ownerToken" type="password" autocomplete="off" aria-label="Owner control token" /></label>
	<p>The token stays in this page's memory.</p><button class="secondary-btn" :disabled="!ownerToken || pending" @click="refresh">Load reports</button></div>
	<p v-if="errorText" role="alert">{{ errorText }}</p>

    <!-- Latest check result -->
    <div v-if="latestReport" class="report-card" :class="'report-' + displayStatus">
      <div class="report-header">
        <span class="status-badge" :class="'status-' + displayStatus">
          {{ displayStatus }}
        </span>
        <span class="report-time">{{ formatTime(latestReport.checked_at) }}</span>
      </div>
	  <p v-if="staleReport">Saved observation is stale or undated. Current state is unknown.</p>
	  <p v-if="latestReport.report.check_error" role="alert">Check: {{ latestReport.report.check_error }}</p>
	  <div v-if="latestReport.report.execution">
		<p>Execution: {{ latestReport.report.execution.status }} · {{ latestReport.report.execution.run_id }}</p>
		<NuxtLink v-if="latestReport.report.execution.status === 'waiting_for_user'" to="/approvals">Review the exact action in Approvals</NuxtLink>
		<button v-if="latestReport.report.input_id" class="secondary-btn" :disabled="checking || !ownerToken" @click="updateCheck">Refresh check</button>
	  </div>
	  <section v-if="latestReport.report.memory" class="memory-diagnostics" aria-label="Memory diagnostics">
		<h3>Memory · {{ latestReport.report.memory.status }}</h3>
		<div class="memory-table"><table class="data-table"><thead><tr><th>Feature</th><th>Configured</th><th>Connected</th><th>Observation</th><th>Reason</th><th>Checked</th></tr></thead><tbody>
		<tr v-for="(c, name) in latestReport.report.memory.components" :key="name"><td>{{ name }}</td><td>{{ c.configured }}</td><td>{{ c.effective }}</td><td>{{ c.status }}</td><td>{{ c.reason }}</td><td>{{ formatTime(c.checked_at) }}</td></tr>
		</tbody></table></div>
		<p>Legacy Redis queue: {{ latestReport.report.memory.legacy_queue?.status }} · depth {{ latestReport.report.memory.legacy_queue?.depth ?? 'unknown' }}</p>
		<p>Durable jobs: {{ latestReport.report.memory.jobs?.status }}</p>
		<template v-if="latestReport.report.memory.jobs?.observation"><p>Pending {{ latestReport.report.memory.jobs.observation.pending }} · running {{ latestReport.report.memory.jobs.observation.running }} · completed {{ latestReport.report.memory.jobs.observation.completed }} · failed {{ latestReport.report.memory.jobs.observation.failed }} · retries {{ latestReport.report.memory.jobs.observation.retries }}</p><p>Oldest pending: {{ latestReport.report.memory.jobs.observation.oldest_pending_seconds ?? 'none' }} seconds</p></template>
		<h4>Index generations · {{ latestReport.report.memory.indexes?.status }}</h4>
		<div v-for="g in latestReport.report.memory.indexes?.generations" :key="g.generation_id"><p>{{ g.generation_id }} · {{ g.scope_type }}:{{ g.scope_id }} · {{ g.state }} · {{ g.active ? 'active' : 'inactive' }} · verified {{ g.verified_at || 'unverified' }}</p><details v-if="g.generation_proof"><summary>Generation evidence (separate from answer quality)</summary><pre>{{ JSON.stringify(g.generation_proof, null, 2) }}</pre></details></div>
		<p v-if="latestReport.report.memory.indexes?.truncated">Showing at most 50 generations.</p>
		<p>Answer quality: {{ latestReport.report.memory.answer_quality?.status || 'unverified' }} · {{ latestReport.report.memory.answer_quality?.reason }}</p>
		<p class="configuration">Configuration: {{ latestReport.report.memory.answer_quality?.configuration }}</p>
	  </section>

      <div v-if="latestReport.report && latestReport.report.checks" class="checks-list">
        <div
          v-for="check in latestReport.report.checks"
          :key="check.name"
          class="check-item"
          :class="'check-' + check.status"
        >
          <span class="check-indicator" :class="'indicator-' + check.status">●</span>
          <span class="check-name">{{ check.name }}</span>
          <span class="check-status">{{ check.status }}</span>
          <span v-if="check.message" class="check-message">{{ check.message }}</span>
          <span class="check-duration">{{ check.duration }}</span>
        </div>
      </div>

      <details v-if="latestReport.report && latestReport.report.config && latestReport.report.config.length > 0" class="config-section">
        <summary>Configuration ({{ latestReport.report.config.length }} keys)</summary>
        <table class="data-table config-table">
          <thead>
            <tr>
              <th>Key</th>
              <th>Value</th>
              <th>Source</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="entry in latestReport.report.config" :key="entry.key">
              <td class="config-key">{{ entry.key }}</td>
              <td class="config-value">{{ entry.effective_value }}</td>
              <td>
                <span class="source-badge" :class="'source-' + entry.source">
                  {{ entry.source || 'default' }}
                </span>
              </td>
              <td>
                <span class="validation-badge" :class="'validation-' + entry.validation_status">
                  {{ entry.validation_status }}
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </details>
    </div>

    <!-- Past reports -->
    <h3 v-if="reports && reports.length > 0" class="section-title">Past Reports</h3>
    <div v-if="pending" class="placeholder-text">Loading reports...</div>
    <div v-else-if="checkError" class="placeholder-text">Failed to load reports.</div>

    <table v-if="reports && reports.length > 0" class="data-table">
      <thead>
        <tr>
          <th>ID</th>
          <th>Status</th>
          <th>Checked At</th>
          <th>Checks</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="report in reports"
          :key="report.id"
          class="report-row"
          :class="{ 'selected-row': latestReport && report.id === latestReport.id }"
          @click="selectReport(report)"
        >
          <td>#{{ report.id }}</td>
          <td>
            <span class="status-badge" :class="'status-' + report.status">
              {{ report.status }}
            </span>
          </td>
          <td>{{ formatTime(report.checked_at) }}</td>
          <td>{{ report.report?.checks?.length || 0 }} checks</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ownerAuthorityRequest } from '~/shared/api/owner-goals'
import type { DoctorReport } from '~/composables/useApi'

useHead({ title: 'Doctor — Butler' })

const ownerToken = ref('')
const reports = ref<DoctorReport[]>([])
const pending = ref(false)
const checkError = ref(false)
const errorText = ref('')
let epoch = 0
let mounted = true
const clock = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => { timer = setInterval(() => { clock.value = Date.now() }, 30000) })
watch(ownerToken, () => { epoch++; latestReport.value = null; reports.value = []; errorText.value = '' }, { flush: 'sync' })
onBeforeUnmount(() => { mounted = false; epoch++; ownerToken.value = ''; clearInterval(timer) })
async function request<T>(path: string, body?: unknown): Promise<T> {
  const proof = { epoch, token: ownerToken.value }
  if (!proof.token) throw new Error('Owner authorization required')
  const result = await ownerAuthorityRequest<T>(path, proof.token, body)
  if (!mounted || proof.epoch !== epoch || proof.token !== ownerToken.value) throw new Error('Owner access changed')
  return result
}
async function refresh() {
  pending.value = true
  try { const r = await request<{ reports: DoctorReport[] }>('/api/v1/doctor/reports'); reports.value = r.reports; checkError.value = false }
  catch { checkError.value = true }
  finally { pending.value = false }
}

const checking = ref(false)
const latestReport = ref<DoctorReport | null>(null)
const staleReport = computed(() => {
  const checked = Date.parse(latestReport.value?.checked_at || '')
  return !Number.isFinite(checked) || clock.value - checked > 300000 || checked > clock.value + 1000
})
const displayStatus = computed(() => staleReport.value ? 'unknown' : latestReport.value?.status || 'unknown')

// Select the latest report when data loads
watch(reports, (newReports) => {
  if (newReports && newReports.length > 0 && !latestReport.value) {
    latestReport.value = newReports[0]
  }
})

function selectReport(report: DoctorReport) {
  latestReport.value = report
}

async function triggerCheck() {
  checking.value = true
  try {
    const result = await request<DoctorReport>('/api/v1/doctor/check', {})
    latestReport.value = result
    await refresh()
  } catch {
    errorText.value = 'Doctor check unavailable. Verify owner access and server diagnostics.'
  } finally {
    checking.value = false
  }
}

async function updateCheck() {
  const id = latestReport.value?.report.input_id
  if (!id) return
  checking.value = true
  try { latestReport.value = await request<DoctorReport>('/api/v1/doctor/check', { input_id: id }); await refresh() }
  catch { errorText.value = 'Check result unavailable.' }
  finally { checking.value = false }
}

function formatTime(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return d.toLocaleString()
}
</script>

<style scoped>
.owner-access {
  display: grid;
  gap: var(--space-3);
  justify-items: start;
  margin-bottom: var(--space-6);
}

.owner-control {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--space-3);
}

.owner-control input {
  padding: var(--space-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  background: var(--color-bg-surface);
  color: var(--color-text-primary);
}

.secondary-btn {
  padding: var(--space-2) var(--space-4);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  background: var(--color-bg-surface);
  color: var(--color-text-primary);
  cursor: pointer;
}

.secondary-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.memory-diagnostics {
  display: grid;
  gap: var(--space-3);
  margin-top: var(--space-5);
}

.memory-diagnostics h3,
.memory-diagnostics h4 {
  font-weight: var(--font-semibold);
}

.memory-table {
  overflow-x: auto;
}

.configuration,
.memory-diagnostics pre {
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-6);
}

.page-header .page-title {
  margin-bottom: 0;
}

.check-btn {
  padding: var(--space-2) var(--space-5);
  border: none;
  border-radius: var(--radius-sm);
  background: var(--color-accent-primary);
  color: var(--color-text-inverse);
  font-size: var(--text-base);
  font-weight: var(--font-semibold);
  cursor: pointer;
  transition: opacity var(--transition-normal);
}

.check-btn:hover:not(:disabled) {
  opacity: 0.85;
}

.check-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.report-card {
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  padding: var(--space-5);
  margin-bottom: var(--space-6);
}

.report-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.report-time {
  color: var(--color-text-secondary);
  font-size: var(--text-sm);
}

.status-badge {
  display: inline-block;
  padding: 3px 10px;
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
  font-weight: var(--font-semibold);
  text-transform: uppercase;
  letter-spacing: var(--tracking-wide);
}

.status-healthy {
  background: var(--color-state-successMuted);
  color: var(--color-state-success);
}

.status-degraded {
  background: var(--color-state-warningMuted);
  color: var(--color-state-warning);
}

.status-unhealthy {
  background: var(--color-state-errorMuted);
  color: var(--color-state-error);
}

.status-unknown {
  background: var(--color-state-neutralMuted);
  color: var(--color-state-neutral);
}

.source-badge {
  display: inline-block;
  padding: 3px 10px;
  border-radius: var(--radius-full);
  font-size: var(--text-xs);
  font-weight: var(--font-semibold);
  text-transform: uppercase;
  letter-spacing: var(--tracking-wide);
}

.source-env {
  background: var(--color-accent-primaryMuted);
  color: var(--color-accent-primary);
}

.source-db {
  background: var(--color-state-successMuted);
  color: var(--color-state-success);
}

.source-default {
  background: var(--color-state-neutralMuted);
  color: var(--color-state-neutral);
}

.checks-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.check-item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  background: var(--color-border-subtle);
  border-radius: var(--radius-sm);
  font-size: var(--text-sm);
}

.check-indicator {
  font-size: 10px;
}

.indicator-healthy { color: var(--color-state-success); }
.indicator-degraded { color: var(--color-state-warning); }
.indicator-unhealthy { color: var(--color-state-error); }

.check-name {
  font-weight: var(--font-semibold);
  min-width: 140px;
}

.check-status {
  min-width: 80px;
  font-size: var(--text-xs);
  text-transform: uppercase;
  letter-spacing: var(--tracking-wide);
}

.check-message {
  color: var(--color-text-secondary);
  flex: 1;
}

.check-duration {
  color: var(--color-text-secondary);
  font-size: var(--text-xs);
  font-family: var(--font-mono);
}

.section-title {
  margin: var(--space-6) 0 var(--space-3);
  font-size: var(--text-lg);
  font-weight: var(--font-semibold);
}

.config-section {
  margin-top: var(--space-4);
}

.config-section summary {
  cursor: pointer;
  font-size: var(--text-sm);
  font-weight: var(--font-semibold);
  color: var(--color-text-secondary);
  margin-bottom: var(--space-2);
}

.config-table {
  font-size: var(--text-xs);
}

.config-key {
  font-family: var(--font-mono);
  font-weight: var(--font-medium);
}

.config-value {
  font-family: var(--font-mono);
  color: var(--color-text-secondary);
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.validation-badge {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 3px;
  font-size: var(--text-xs);
  font-weight: var(--font-semibold);
  text-transform: uppercase;
}

.validation-ok {
  background: var(--color-state-successMuted);
  color: var(--color-state-success);
}

.validation-missing {
  background: var(--color-state-warningMuted);
  color: var(--color-state-warning);
}

.validation-invalid {
  background: var(--color-state-errorMuted);
  color: var(--color-state-error);
}

.report-row {
  cursor: pointer;
  transition: background var(--transition-normal);
}

.report-row:hover {
  background: var(--color-border-subtle);
}

.selected-row {
  background: var(--color-accent-primaryMuted);
}
</style>
