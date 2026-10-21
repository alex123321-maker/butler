<template><div class="mind-advanced">
  <main class="page">

    <AppPanel title="Owner access">
      <label>Owner control token <input v-model="token" type="password" autocomplete="off" aria-label="Owner control token" /></label>
      <p>Authenticate to inspect observations or configure checks. Access lasts while this page is open.</p>
      <AppButton :disabled="!token || pending" @click="load">Load initiatives</AppButton>
      <AppAlert v-if="error" tone="error">{{ error }}</AppAlert>
      <AppAlert v-if="saved" tone="info">{{ saved }}</AppAlert>
    </AppPanel>
    <template v-if="view">
      <AppPanel title="Observed work">
        <p>{{ view.attention_used_today }} of {{ view.attention_daily_limit }} initiative tasks admitted today. Empty checks do not send notifications.</p>
        <article v-for="candidate in view.initiatives" :key="candidate.initiative_id" class="entry">
          <strong>{{ projectName(candidate.source_id) }}</strong>
          <p>{{ statusLabel(candidate.status) }} · {{ reasonLabel(candidate.reason) }}</p>
          <p>{{ candidate.occurrences }} distinct observations · confidence {{ candidate.confidence }}% · importance {{ candidate.importance }}%</p>
          <p>Estimated model calls {{ candidate.expected_model_calls }} · attention {{ candidate.attention_cost }}</p>
          <p v-if="candidate.status === 'deferred'">Next consideration {{ displayTime(candidate.not_before) }}</p>
          <NuxtLink v-if="candidate.task_id" :to="`/tasks/${candidate.task_id}`">Open admitted task</NuxtLink>
        </article>
        <p v-if="!view.initiatives.length">No proposed or admitted work.</p>
      </AppPanel>
      <AppPanel title="Checks and active hours">
        <article v-for="source in view.sources" :key="source.source_id" class="entry">
          <strong>{{ source.project_ref }}</strong>
          <p>{{ source.enabled ? 'Enabled' : 'Paused' }} · {{ source.time_zone }} · active {{ source.active_start_hour }}:00–{{ source.active_end_hour }}:00</p>
          <p>{{ source.task_goal }}</p>
          <p>Repeated failure threshold {{ source.failure_threshold }} · minimum confidence {{ source.confidence_threshold }}% · cooldown {{ source.cooldown_seconds }} seconds</p>
          <p>Notifications: {{ source.digest_channel ? `Grouped ${source.digest_channel} notices` : 'Owner view only' }}</p>
          <AppButton :disabled="pending" @click="setSourceEnabled(source, !source.enabled)">{{ source.enabled ? 'Pause checks' : 'Enable checks' }}</AppButton>
        </article>
        <p v-if="!view.sources.length">No sources configured. A confirmed goal and active mandate are required to admit work.</p>
        <NuxtLink to="/goals">Review goals and autonomy</NuxtLink>
        <form class="config-form" @submit.prevent="createSource">
          <h3>Add project checks</h3>
          <label>Active mandate <select v-model="selectedMandate" required aria-label="Active mandate"><option value="" disabled>Select mandate</option><option v-for="mandate in mandates" :key="mandate.mandate_id" :value="mandate.mandate_id">{{ goalTitle(mandate.goal_id) }} · {{ mandate.purpose }}</option></select></label>
          <label>Observation <select v-model="observerKind" aria-label="Observation"><option value="task_project">Failed project task attempts</option><option value="broker_project">Recorded checks from a reviewed connector</option><option value="mobile_inbox">Android messages: autonomous assistance</option></select></label>
          <p v-if="observerKind === 'mobile_inbox'">Read messages from apps permitted in Companion. Every 20 new messages or after one hour, Butler can remember useful information, search earlier messages and hide repetitive noise. A permanent read and memory preparation mandate avoids per-message approvals. The schedule is created automatically.</p>
          <template v-if="observerKind === 'broker_project'">
            <label>Reviewed check <select v-model="selectedCheck" required aria-label="Reviewed check"><option value="" disabled>Select read operation</option><option v-for="check in reviewedChecks" :key="check.key" :value="check.key">{{ check.label }}</option></select></label>
            <label>Exact check URL <input v-model="checkResource" type="url" required aria-label="Exact check URL" /></label>
            <p>Observe saved, authorized checks at this exact address. A schedule reads their results and does not make new external requests.</p>
            <NuxtLink to="/extensions">Review installed connectors</NuxtLink>
          </template>
          <label v-if="observerKind !== 'mobile_inbox'">Project reference <input v-model="project" required maxlength="200" aria-label="Project reference" /></label>
          <label v-if="observerKind !== 'mobile_inbox'">Project revision <input v-model="projectRevision" required pattern="[a-f0-9]{64}" maxlength="64" aria-label="Project revision" /></label>
          <label>Conversation reference <input v-model="sessionKey" required maxlength="200" aria-label="Conversation reference" /></label>
          <label v-if="observerKind !== 'mobile_inbox'">Task to prepare <textarea v-model="taskGoal" required maxlength="8192" rows="3" aria-label="Task to prepare" /></label>
          <label v-if="observerKind !== 'mobile_inbox'">Acceptance criteria, one per line <textarea v-model="criteria" required rows="3" aria-label="Acceptance criteria" /></label>
          <label>Time zone <input v-model="timeZone" required aria-label="Time zone" /></label>
          <div v-if="observerKind !== 'mobile_inbox'" class="fields"><label>Active from <input v-model.number="activeStart" type="number" min="0" max="23" required aria-label="Active from" /></label><label>Active until <input v-model.number="activeEnd" type="number" min="0" max="23" required aria-label="Active until" /></label></div>
          <label v-if="observerKind !== 'mobile_inbox'">Repeated failures <input v-model.number="failures" type="number" min="1" max="100" required aria-label="Repeated failures" /></label>
          <label v-if="observerKind !== 'mobile_inbox'">Action policy <select v-model="policy"><option value="prepare_only">Local preparation only</option><option value="normal">Actions within current mandate</option></select></label>
          <label v-if="observerKind !== 'mobile_inbox'">Notifications <select v-model="digestChannel"><option value="">Owner view only</option><option value="web">Grouped Web notices</option><option value="telegram">Grouped Telegram notices</option></select></label>
          <p>Start with paused checks. Enable them after reviewing the configuration. Notification channel must match the selected conversation.</p>
          <AppButton type="submit" :disabled="pending || !selectedMandate">Save paused checks</AppButton>
        </form>
      </AppPanel>
      <AppPanel title="Schedules">
        <article v-for="schedule in view.schedules" :key="schedule.schedule_id" class="entry">
          <strong>{{ projectName(schedule.source_id) }}</strong>
          <p>{{ schedule.kind === 'daily' ? `Daily at ${String(schedule.hour).padStart(2, '0')}:${String(schedule.minute).padStart(2, '0')}` : `Every ${schedule.interval_seconds} seconds` }} · {{ schedule.time_zone }}</p>
          <p>{{ schedule.enabled ? 'Enabled' : 'Paused' }} · next {{ displayTime(schedule.next_at) }} · {{ schedule.missed_policy === 'coalesce' ? 'Combine missed checks into one' : 'Skip missed checks' }}</p>
          <AppButton :disabled="pending" @click="setScheduleEnabled(schedule, !schedule.enabled)">{{ schedule.enabled ? 'Pause schedule' : 'Enable schedule' }}</AppButton>
        </article>
        <form v-if="view.sources.length" class="config-form" @submit.prevent="createSchedule">
          <h3>Add a schedule</h3>
          <label>Project checks <select v-model="scheduleSource" required aria-label="Project checks"><option value="" disabled>Select project</option><option v-for="source in view.sources" :key="source.source_id" :value="source.source_id">{{ source.project_ref }}</option></select></label>
          <label>Frequency <select v-model="scheduleKind"><option value="interval">Interval</option><option value="daily">Daily</option></select></label>
          <label v-if="scheduleKind === 'interval'">Interval in minutes <input v-model.number="intervalMinutes" type="number" min="1" max="10080" required aria-label="Interval in minutes" /></label>
          <label v-else>Local time <input v-model="dailyTime" type="time" required aria-label="Local time" /></label>
          <label>After downtime <select v-model="missedPolicy"><option value="coalesce">Perform one current check</option><option value="skip">Skip missed checks</option></select></label>
          <AppButton type="submit" :disabled="pending || !scheduleSource">Save schedule</AppButton>
        </form>
      </AppPanel>
      <AppPanel title="Recent check decisions">
        <p v-for="event in view.events" :key="event.source_id + event.event_id" class="entry">{{ projectName(event.source_id) }} · {{ reasonLabel(event.verdict) }} · {{ displayTime(event.observed_at) }}</p>
        <p v-if="!view.events.length">No observations recorded.</p>
      </AppPanel>
      <AppPanel title="Grouped notifications">
        <article v-for="notice in view.digests" :key="notice.notification_id" class="entry"><p>{{ notice.content }}</p><p>{{ notice.channel }} · {{ statusLabel(notice.status) }} · {{ notice.attempt_count }} send attempts<span v-if="notice.error_code"> · {{ reasonLabel(notice.error_code) }}</span></p></article>
        <p v-if="!view.digests.length">No grouped notifications. Empty checks and proposals remain quiet.</p>
      </AppPanel>
    </template>
  </main>
</div></template>

<script setup lang="ts">
const mindPanelProps=defineProps<{ownerToken?:string}>()
import AppAlert from '~/shared/ui/AppAlert.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppPanel from '~/shared/ui/AppPanel.vue'
import { ownerAuthorityRequest, type OwnerAuthorityView } from '~/shared/api/owner-goals'
import type { InitiativeView, InitiativeSource, InitiativeSchedule } from '~/shared/api/initiatives'

const token = ref(mindPanelProps.ownerToken || ''); const pending = ref(false); const error = ref(''); const saved = ref('')
const view = ref<InitiativeView | null>(null); const authority = ref<OwnerAuthorityView | null>(null)
const selectedMandate = ref(''); const project = ref(''); const projectRevision = ref(''); const sessionKey = ref(''); const taskGoal = ref(''); const criteria = ref('')
const timeZone = ref('UTC'); const activeStart = ref(9); const activeEnd = ref(20); const failures = ref(3); const policy = ref('prepare_only'); const digestChannel = ref('')
interface ReviewedCheckList { items?: { state: { manifest_hash: string; reviewed: boolean }; manifest: { extension_id?: string; id: string; version: string; operations: { name: string; method: string; effect: string }[] } }[] }
const connectorView = ref<ReviewedCheckList | null>(null)
const observerKind = ref('task_project'); const selectedCheck = ref(''); const checkResource = ref('')
const reviewedChecks = computed(() => connectorView.value?.items?.filter(item => item.state.reviewed).flatMap(item => item.manifest.operations.filter(op => op.method === 'GET' && op.effect === 'read').map(op => ({ key: `${item.state.manifest_hash}:${op.name}`, hash: item.state.manifest_hash, tool: op.name, label: `${item.manifest.id} ${item.manifest.version} · ${op.name}` }))) || [])
const scheduleSource = ref(''); const scheduleKind = ref('interval'); const intervalMinutes = ref(60); const dailyTime = ref('09:00'); const missedPolicy = ref('coalesce')
const mandates = computed(() => authority.value?.mandates.filter(m => m.status === 'active' && m.allow_initiative && authority.value?.goals.some(g => g.goal_id === m.goal_id && g.revision === m.goal_revision && g.kind === 'objective' && g.status === 'active')) || [])
const goalTitle = (id: string) => authority.value?.goals.find(g => g.goal_id === id)?.title || id
const projectName = (id: string) => view.value?.sources.find(s => s.source_id === id)?.project_ref || 'Configured source'
const displayTime = (value?: string) => value ? new Date(value).toLocaleString() : 'Not scheduled'
const labels: Record<string, string> = { observing: 'Observing', proposed: 'Proposed', ready: 'Ready for admission', deferred: 'Deferred', promoted: 'Task admitted', completed: 'Verified completion', rejected: 'Rejected', suppressed: 'Suppressed', result_ready: 'Available in owner view', delivered: 'Accepted by channel', delivery_unknown: 'Channel acceptance uncertain', delivery_failed: 'Delivery stopped', queued: 'Queued', sending: 'Sending', repeat_threshold_not_met: 'Waiting for repeated failures', confidence_insufficient: 'Confidence below threshold', importance_insufficient: 'Importance below threshold', outside_active_hours: 'Outside active hours', source_cooldown: 'Check cooldown', authority_unavailable: 'Current mandate unavailable', owner_day_budget_exhausted: 'Daily execution budget exhausted', execution_budget_exhausted: 'Execution budget exhausted', condition_obsolete: 'Condition changed or resolved', source_changed: 'Configuration changed', own_change: 'Change from Butler work', condition_cleared: 'Condition resolved', empty_check: 'No useful change', duplicate_evidence: 'Previously observed evidence', already_admitted: 'Work already admitted', stale_observation: 'Observation expired', merged_observation: 'Combined observation', useful_observation: 'Useful condition observed', ordinary_task_admitted: 'Admitted as an ordinary task' }
const reasonLabel = (value: string) => labels[value] || value.replaceAll('_', ' ')
const statusLabel = reasonLabel
onBeforeUnmount(() => { token.value = ''; view.value = null; authority.value = null; connectorView.value = null })
watch(token, () => { view.value = null; authority.value = null; connectorView.value = null; saved.value = '' })
const refresh = async () => { const access = token.value; const [nextView, nextAuthority, nextConnectors] = await Promise.all([ownerAuthorityRequest<InitiativeView>('/api/v3/initiatives', access), ownerAuthorityRequest<OwnerAuthorityView>('/api/v3/owner-authority', access), ownerAuthorityRequest<ReviewedCheckList>('/api/v4/extensions', access).catch(() => null)]); if (access !== token.value) return; view.value = nextView; authority.value = nextAuthority; connectorView.value = nextConnectors }
const load = async () => { pending.value = true; error.value = ''; saved.value = ''; try { await refresh() } catch { view.value = null; authority.value = null; error.value = 'Initiatives are unavailable. Check owner access and reload.' } finally { pending.value = false } }
const apply = async (path: string, body: unknown) => { pending.value = true; error.value = ''; saved.value = ''; try { await ownerAuthorityRequest(path, token.value, body); await refresh(); saved.value = 'Configuration saved.' } catch { error.value = 'Change was rejected. Reload and review current configuration or authority.' } finally { pending.value = false } }
const setSourceEnabled = (source: InitiativeSource, enabled: boolean) => apply('/api/v3/initiatives/sources', { expected_revision: source.revision, source: { ...source, enabled } })
const setScheduleEnabled = (schedule: InitiativeSchedule, enabled: boolean) => apply('/api/v3/initiatives/schedules', { expected_revision: schedule.revision, schedule: { ...schedule, enabled } })
const createMobileSource = () => {
  const mandate = mandates.value.find(m => m.mandate_id === selectedMandate.value)
  if (!mandate) return
  return apply('/api/v3/initiatives/sources', { expected_revision: 0, source: { source_id: `mobile-${crypto.randomUUID()}`, goal_id: mandate.goal_id, mandate_id: mandate.mandate_id, session_key: sessionKey.value, observer_kind: 'mobile_inbox', purpose: mandate.purpose, time_zone: timeZone.value, enabled: false } })
}
const createSource = () => { if (observerKind.value === 'mobile_inbox') return createMobileSource(); const mandate = mandates.value.find(m => m.mandate_id === selectedMandate.value); const check = reviewedChecks.value.find(c => c.key === selectedCheck.value); if (!mandate || activeStart.value === activeEnd.value || (observerKind.value === 'broker_project' && !check)) { error.value = 'Select a current mandate, reviewed check when needed, and different active hours.'; return } return apply('/api/v3/initiatives/sources', { expected_revision: 0, source: { source_id: `project-${crypto.randomUUID()}`, goal_id: mandate.goal_id, mandate_id: mandate.mandate_id, session_key: sessionKey.value, project_ref: project.value, project_revision: projectRevision.value, observer_kind: observerKind.value, ...(observerKind.value === 'broker_project' && check ? { check_manifest_hash: check.hash, check_tool_name: check.tool, check_resource: checkResource.value } : {}), digest_channel: digestChannel.value, purpose: mandate.purpose, time_zone: timeZone.value, task_goal: taskGoal.value, execution_policy: policy.value, completion_criteria: criteria.value.split('\n').map(c => c.trim()).filter(Boolean), active_start_hour: activeStart.value, active_end_hour: activeEnd.value, failure_threshold: failures.value, confidence_threshold: 90, minimum_importance: 60, cooldown_seconds: 3600, enabled: false } }) }
const createSchedule = () => { const source = view.value?.sources.find(s => s.source_id === scheduleSource.value); if (!source) return; const [hour, minute] = dailyTime.value.split(':').map(Number); return apply('/api/v3/initiatives/schedules', { expected_revision: 0, schedule: { schedule_id: `schedule-${crypto.randomUUID()}`, source_id: source.source_id, time_zone: source.time_zone, kind: scheduleKind.value, hour: scheduleKind.value === 'daily' ? hour : 0, minute: scheduleKind.value === 'daily' ? minute : 0, interval_seconds: scheduleKind.value === 'interval' ? intervalMinutes.value * 60 : 0, missed_policy: missedPolicy.value, grace_seconds: 60, enabled: true } }) }
</script>
