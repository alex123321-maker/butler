<template><div class="mind-advanced">
  <div class="page">
    <div class="page-header">

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
</div></template>

<script setup lang="ts">
const mindPanelProps=defineProps<{ownerToken?:string}>()
import { ref } from 'vue'
import { ownerAuthorityRequest } from '~/shared/api/owner-goals'
import type { DoctorReport } from '~/composables/useApi'

const ownerToken = ref(mindPanelProps.ownerToken || '')
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
