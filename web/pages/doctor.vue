<template>
  <div class="page">
    <div class="page-header">
      <h2 class="page-title">Doctor</h2>
      <button class="check-btn" :disabled="checking" @click="triggerCheck">
        {{ checking ? 'Running...' : 'Run System Check' }}
      </button>
    </div>

    <!-- Latest check result -->
    <div v-if="latestReport" class="report-card" :class="'report-' + latestReport.status">
      <div class="report-header">
        <span class="status-badge" :class="'status-' + latestReport.status">
          {{ latestReport.status }}
        </span>
        <span class="report-time">{{ formatTime(latestReport.checked_at) }}</span>
      </div>

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
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="entry in latestReport.report.config" :key="entry.key">
              <td class="config-key">{{ entry.key }}</td>
              <td class="config-value">{{ entry.effective_value }}</td>
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
import { useDoctorReports, useDoctorCheck } from '~/composables/useApi'
import type { DoctorReport } from '~/composables/useApi'

useHead({ title: 'Doctor — Butler' })

const { data: reports, pending, error: checkError, refresh } = useDoctorReports()
const { runCheck } = useDoctorCheck()

const checking = ref(false)
const latestReport = ref<DoctorReport | null>(null)

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
    const result = await runCheck()
    latestReport.value = result
    await refresh()
  } catch (e) {
    console.error('Doctor check failed:', e)
  } finally {
    checking.value = false
  }
}

function formatTime(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return d.toLocaleString()
}
</script>

<style scoped>
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24px;
}

.page-header .page-title {
  margin-bottom: 0;
}

.check-btn {
  padding: 8px 20px;
  border: none;
  border-radius: 6px;
  background: var(--color-primary, #4f8cff);
  color: #fff;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.15s;
}

.check-btn:hover:not(:disabled) {
  opacity: 0.85;
}

.check-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.report-card {
  background: var(--color-surface, #1e1e2e);
  border: 1px solid var(--color-border, #2d2d3d);
  border-radius: 8px;
  padding: 20px;
  margin-bottom: 24px;
}

.report-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.report-time {
  color: var(--color-text-secondary, #888);
  font-size: 13px;
}

.status-badge {
  display: inline-block;
  padding: 3px 10px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

.status-healthy {
  background: rgba(46, 204, 113, 0.15);
  color: #2ecc71;
}

.status-degraded {
  background: rgba(243, 156, 18, 0.15);
  color: #f39c12;
}

.status-unhealthy {
  background: rgba(231, 76, 60, 0.15);
  color: #e74c3c;
}

.status-unknown {
  background: rgba(149, 165, 166, 0.15);
  color: #95a5a6;
}

.checks-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.check-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  background: rgba(255, 255, 255, 0.03);
  border-radius: 6px;
  font-size: 13px;
}

.check-indicator {
  font-size: 10px;
}

.indicator-healthy { color: #2ecc71; }
.indicator-degraded { color: #f39c12; }
.indicator-unhealthy { color: #e74c3c; }

.check-name {
  font-weight: 600;
  min-width: 140px;
}

.check-status {
  min-width: 80px;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

.check-message {
  color: var(--color-text-secondary, #888);
  flex: 1;
}

.check-duration {
  color: var(--color-text-secondary, #888);
  font-size: 12px;
  font-family: monospace;
}

.section-title {
  margin: 24px 0 12px;
  font-size: 16px;
  font-weight: 600;
}

.config-section {
  margin-top: 16px;
}

.config-section summary {
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-secondary, #888);
  margin-bottom: 8px;
}

.config-table {
  font-size: 12px;
}

.config-key {
  font-family: monospace;
  font-weight: 500;
}

.config-value {
  font-family: monospace;
  color: var(--color-text-secondary, #888);
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.validation-badge {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 3px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
}

.validation-ok {
  background: rgba(46, 204, 113, 0.15);
  color: #2ecc71;
}

.validation-missing {
  background: rgba(243, 156, 18, 0.15);
  color: #f39c12;
}

.validation-invalid {
  background: rgba(231, 76, 60, 0.15);
  color: #e74c3c;
}

.report-row {
  cursor: pointer;
  transition: background 0.15s;
}

.report-row:hover {
  background: rgba(255, 255, 255, 0.04);
}

.selected-row {
  background: rgba(79, 140, 255, 0.08);
}
</style>
