<template><div class="mind-advanced">
  <main class="page">

    <AppPanel title="Owner access">
      <label>Owner control token <input v-model="token" type="password" autocomplete="off" aria-label="Owner control token" /></label>
      <p>The token stays in this page's memory.</p>
      <AppButton :disabled="!token || pending" @click="load">Load skills</AppButton>
      <AppAlert v-if="error" tone="error">{{ error }}</AppAlert><AppAlert v-if="receipt" tone="info">{{ receipt }}</AppAlert>
    </AppPanel>
    <template v-if="loaded">
      <AppPanel title="Stage a reviewed procedure">
        <label>Procedure manifest JSON <textarea v-model="manifest" rows="10" aria-label="Procedure manifest JSON" /></label>
        <p>Use exact task result references, dependent tool versions, failure checks and stop rules. Source URLs record provenance; no URL is downloaded.</p>
        <AppButton :disabled="pending || !manifest" @click="stage">Stage candidate</AppButton>
      </AppPanel>
      <AppPanel title="Versions and history">
        <article v-for="item in items" :key="item.digest" class="skill-row">
          <div><strong>{{ item.skill_id }}</strong><p>{{ item.version }} · {{ item.status }} · generation {{ item.generation }}</p><p>{{ item.body_available ? item.package.manifest.description : 'Source deleted, stale or unavailable. Procedure text cannot be loaded.' }}</p><p class="digest">SHA-256 {{ item.digest }}</p></div>
          <AppButton :disabled="pending" @click="select(item)">Review {{ item.skill_id }} {{ item.version }}</AppButton>
        </article><p v-if="!items.length">No reviewed procedures.</p>
      </AppPanel>
      <AppPanel v-if="selected" :title="`${selected.skill_id} ${selected.version}`">
        <template v-if="selected.body_available">
          <p>Source {{ selected.package.manifest.source.uri }} · {{ selected.package.manifest.source.revision }}</p>
          <h3>When to use</h3><ul><li v-for="value in selected.package.manifest.when" :key="value">{{ value }}</li></ul>
          <h3>Do not use when</h3><ul><li v-for="value in selected.package.manifest.not_applicable" :key="value">{{ value }}</li></ul>
          <ol><li v-for="step in selected.package.manifest.steps" :key="step.id"><strong>{{ step.tool }}</strong> {{ step.intent }}<p>Check: {{ step.success_check }} · on error: {{ step.on_error }} · retry: {{ step.retry }} · approval checkpoint: {{ step.approval_checkpoint ? 'required' : 'ordinary action policy' }}</p></li></ol>
          <div class="actions"><AppButton :disabled="pending || selected.status === 'revoked'" @click="action('test')">Validate evidence and contracts</AppButton><AppButton :disabled="pending || selected.active || !['tested', 'obsolete'].includes(selected.status)" @click="action(selected.status === 'obsolete' ? 'rollback' : 'activate')">{{ selected.status === 'obsolete' ? 'Roll back to this version' : 'Activate tested version' }}</AppButton></div>
        </template>
        <div class="actions"><AppButton :disabled="pending || !selected.active" @click="action('disable')">Disable version</AppButton><AppButton :disabled="pending || selected.status === 'revoked'" @click="action('revoke')">Permanently revoke version</AppButton><AppButton :disabled="pending" @click="usage">Load usage history</AppButton><AppButton :disabled="pending || !selected.body_available" @click="quality">Compare observed outcomes</AppButton></div>
        <p v-if="checks.length">Independent checks: {{ checks.join(', ') }}</p>
        <ul v-if="usages"><li v-for="entry in usages" :key="entry.run_id"><NuxtLink :to="`/tasks/${encodeURIComponent(entry.task_id)}`">{{ entry.task_id }}</NuxtLink> · revision {{ entry.task_revision }} · run {{ entry.run_id }}</li></ul>
        <p v-if="report">Verified outcomes {{ report.verified_successes }} · unverified attempts {{ report.unverified_attempts }} · known tool failures {{ report.known_tool_failures }}. Source trials and measured usage are available below; missing usage is not zero cost.</p>
        <pre v-if="report">{{ JSON.stringify(report, null, 2) }}</pre>
        <label>Bounded descriptive correction grant JSON <textarea v-model="grant" rows="4" aria-label="Descriptive correction grant JSON" /></label>
        <p>Bind one active parent digest to an existing mandate revision, purpose and expiry. Descriptive corrections cannot change steps, checks, sources, dependencies, retries or permissions.</p>
        <AppButton :disabled="pending || !selected.active || !grant" @click="grantCorrection">Save exact correction grant</AppButton>
        <label>Correction grant ID <input v-model="grantID" aria-label="Correction grant ID" /></label><AppButton :disabled="pending || !grantID" @click="revokeGrant">Revoke correction grant</AppButton>
      </AppPanel>
    </template>
  </main>
</div></template>

<script setup lang="ts">
const mindPanelProps=defineProps<{ownerToken?:string}>()
import AppPanel from '~/shared/ui/AppPanel.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppAlert from '~/shared/ui/AppAlert.vue'
import { skillRequest, type SkillRecord, type SkillResponse } from '~/shared/api/skills'

const token = ref(mindPanelProps.ownerToken || ''), manifest = ref(''), grant = ref(''), grantID = ref(''), error = ref(''), receipt = ref('')
const loaded = ref(false), pending = ref(false), items = ref<SkillRecord[]>([]), selected = ref<SkillRecord>()
const checks = ref<string[]>([]), usages = ref<SkillResponse['usages']>(), report = ref<SkillResponse['quality']>()
watch(token, () => { loaded.value = false; items.value = []; selected.value = undefined; manifest.value = ''; grant.value = ''; grantID.value = ''; checks.value = []; usages.value = undefined; report.value = undefined; receipt.value = ''; error.value = '' })
async function load() {
  const requestToken = token.value
  pending.value = true; error.value = ''
  try { const response = await skillRequest(requestToken); if (requestToken !== token.value) return; items.value = response.items || []; loaded.value = true; if (selected.value) selected.value = items.value.find(item => item.digest === selected.value?.digest) }
  catch { error.value = 'Owner access or reviewed skill service unavailable.' } finally { pending.value = false }
}
function select(item: SkillRecord) { selected.value = item; checks.value = []; usages.value = undefined; report.value = undefined; grant.value = ''; grantID.value = '' }
async function control(request: Record<string, unknown>, message: string) {
  const requestToken = token.value
  pending.value = true; error.value = ''; receipt.value = ''
  try { const response = await skillRequest(requestToken, request); if (requestToken !== token.value) return; if (response.error) throw new Error(); receipt.value = message; if (response.validation) checks.value = response.validation.checks; if (response.usages) usages.value = response.usages; if (response.quality) report.value = response.quality; await load() }
  catch { error.value = 'Action denied or source/version changed. Reload and review current state.' } finally { pending.value = false }
}
async function stage() { await control({ operation: 'stage', manifest_text: manifest.value }, 'Candidate stored with verified source links. Activation requires separate review.') }
async function action(operation: string) { if (selected.value) await control({ operation, digest: selected.value.digest, generation: selected.value.generation }, 'Procedure lifecycle decision recorded for the exact version.') }
async function usage() { if (selected.value) await control({ operation: 'usages', digest: selected.value.digest }, 'Usage history loaded.') }
async function quality() { if (selected.value) await control({ operation: 'quality', digest: selected.value.digest }, 'Observed outcomes loaded. Creation is not a success metric.') }
async function grantCorrection() { if (!selected.value) return; try { await control({ operation: 'grant_correction', grant: { ...JSON.parse(grant.value), skill_id: selected.value.skill_id, parent_digest: selected.value.digest, class: 'descriptive_correction_v1' } }, 'Bounded correction grant saved.') } catch { error.value = 'Grant must be valid JSON.' } }
async function revokeGrant() { await control({ operation: 'revoke_correction', grant: { grant_id: grantID.value } }, 'Correction grant revoked.') }
</script>
