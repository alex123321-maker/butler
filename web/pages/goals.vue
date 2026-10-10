<template>
  <main class="page">
    <section class="page-header">
      <div><h2 class="page-title">Goals and autonomy</h2><p class="page-copy">Confirmed owner intent, bounded permissions and current obligations.</p></div>
    </section>
    <AppPanel title="Owner access">
      <label>Owner control token <input v-model="token" type="password" autocomplete="off" aria-label="Owner control token" /></label>
      <p>The token stays in this page's memory. Authenticate to read or change personal authority.</p>
      <AppButton :disabled="!token || pending" @click="load">Load owner authority</AppButton>
      <AppAlert v-if="error" tone="error">{{ error }}</AppAlert>
      <AppAlert v-if="receipt" tone="info">Authority version {{ receipt.revision }} saved. Branch propagation: {{ receipt.propagation }}. Previous effects remain in history.</AppAlert>
    </AppPanel>
    <template v-if="view">
      <AppAlert v-if="view.pending_propagation" tone="warning">{{ view.pending_propagation }} authority changes await branch cancellation. New actions under old authority are already denied.</AppAlert>
      <AppPanel title="Owner intent">
        <div v-for="goal in view.goals" :key="goal.goal_id" class="authority-row">
          <div><strong>{{ goal.title }}</strong><p>{{ goal.kind }} · {{ goal.status }} · version {{ goal.revision }}</p><p>{{ goal.purpose }} · {{ goal.source?.author }} via {{ goal.source?.channel }}</p><ul><li v-for="constraint in goal.constraints" :key="constraint">{{ constraint }}</li></ul></div>
          <div class="actions">
            <AppButton :disabled="pending || goal.status === 'revoked'" @click="editGoal(goal)">Edit goal</AppButton>
            <AppButton v-if="goal.kind === 'assumption'" :disabled="pending" @click="confirmAssumption(goal)">Confirm as new objective</AppButton>
            <AppButton v-if="goal.kind !== 'assumption'" :disabled="pending || goal.status === 'revoked'" @click="changeGoalStatus(goal, 'paused')">Pause goal</AppButton>
            <AppButton v-if="goal.kind !== 'assumption'" variant="danger" :disabled="pending || goal.status === 'revoked'" @click="changeGoalStatus(goal, 'revoked')">Revoke goal</AppButton>
          </div>
        </div>
        <p v-if="!view.goals.length">No owner intent recorded.</p>
        <form class="authority-form" @submit.prevent="saveGoal">
          <h3>{{ goalEditId ? 'Amend owner intent' : 'Record owner intent' }}</h3>
          <label>Intent kind <select v-model="goalKind" :disabled="Boolean(goalEditId)"><option value="objective">Confirmed objective</option><option value="preference">Preference</option><option value="constraint">Constraint</option><option value="assumption">Unconfirmed assumption</option></select></label>
          <label>Goal title <input v-model="goalTitle" required aria-label="Goal title" /></label>
          <label>Purpose <input v-model="goalPurpose" required aria-label="Purpose" placeholder="service_maintenance" /></label>
          <label>Constraints, one per line <textarea v-model="goalConstraints" rows="3" /></label>
          <label>Goal deadline <input v-model="goalDeadline" required type="datetime-local" aria-label="Goal deadline" /></label>
          <p v-if="parentRef">Explicit confirmation of {{ parentRef }} creates separate confirmed authority.</p>
          <div class="actions"><AppButton :disabled="pending" type="submit">Save owner intent</AppButton><AppButton variant="secondary" type="button" @click="resetGoal">New intent</AppButton></div>
        </form>
      </AppPanel>
      <AppPanel title="Autonomy mandates">
        <div v-for="mandate in view.mandates" :key="mandate.mandate_id" class="authority-row">
          <div><strong>{{ mandate.purpose }}</strong><p>{{ mandate.status }} · version {{ mandate.revision }} · goal {{ mandate.goal_id }} version {{ mandate.goal_revision }}</p><p>{{ mandate.tools.join(', ') }} · {{ mandate.effects.join(', ') }} · {{ mandate.privacy }}</p><p v-for="resource in mandate.resource_scopes" :key="resource.origin + resource.path_prefix">{{ resource.origin }}{{ resource.path_prefix }}</p><p>Model turns {{ mandate.budget.model_turns }} · tokens {{ mandate.budget.tokens }} · external calls {{ mandate.budget.external_calls }} · children {{ mandate.budget.children }}</p><p>Deadline {{ mandate.expires_at }} · budget period ends {{ mandate.budget_deadline || mandate.expires_at }} · reports {{ mandate.reporting }}</p><p>Granted by {{ mandate.source?.author }} via {{ mandate.source?.channel }}</p></div>
          <div class="actions"><AppButton :disabled="pending || mandate.status === 'revoked'" @click="changeMandateStatus(mandate, 'paused')">Pause mandate</AppButton><AppButton variant="danger" :disabled="pending || mandate.status === 'revoked'" @click="changeMandateStatus(mandate, 'revoked')">Revoke mandate</AppButton></div>
        </div>
        <form class="authority-form" @submit.prevent="saveMandate">
          <h3>Grant bounded autonomy</h3>
          <label>Confirmed objective <select v-model="mandateGoalId" required aria-label="Confirmed objective"><option value="" disabled>Select objective</option><option v-for="goal in objectives" :key="goal.goal_id" :value="goal.goal_id">{{ goal.title }} · version {{ goal.revision }}</option></select></label>
          <label>Resource origin <input v-model="origin" type="url" required aria-label="Resource origin" placeholder="https://test.example.invalid" /></label>
          <label>Account or service path <input v-model="pathPrefix" required aria-label="Account or service path" placeholder="/test-service" /></label>
          <label>Allowed tools, comma separated <input v-model="tools" required aria-label="Allowed tools" placeholder="http.request" /></label>
          <label>Action classes <select v-model="effects" multiple aria-label="Action classes"><option value="read">Read</option><option value="prepare">Prepare</option><option value="commit">External changes</option><option value="unknown">Explicitly allow opaque effects</option></select></label>
          <label>Model privacy <select v-model="privacy"><option value="local_only">Local models only</option><option value="cloud_allowed">Allow cloud model context</option></select></label>
          <label>Credential alias <input v-model="credentialAlias" placeholder="Optional metadata alias" /></label><label>Credential field <input v-model="credentialField" placeholder="password" /></label>
          <label>Mandate deadline <input v-model="mandateDeadline" required type="datetime-local" aria-label="Mandate deadline" /></label>
          <label v-for="limit in budgetLimits" :key="limit.key">{{ limit.label }} <input v-model.number="budget[limit.key]" type="number" min="0" max="1000000000000" required :aria-label="limit.label" /></label>
          <label><input v-model="allowInitiative" type="checkbox" /> Allow Butler to initiate tasks within this scope</label>
          <label>Reporting <select v-model="reporting"><option value="exceptions">Exceptions</option><option value="each_result">Each saved result</option><option value="daily">Daily</option></select></label>
          <AppButton type="submit" :disabled="pending || !mandateGoalId">Grant mandate</AppButton>
        </form>
      </AppPanel>
      <AppPanel title="Current obligations">
        <div v-for="obligation in view.obligations" :key="obligation.task_id" class="authority-row"><div><NuxtLink :to="`/tasks/${obligation.task_id}`">{{ obligation.task_id }}</NuxtLink><p>{{ obligation.initiative_reason }}</p><p>Goal {{ obligation.goal_id }} version {{ obligation.goal_revision }} · mandate {{ obligation.mandate_id }} version {{ obligation.mandate_revision }}</p><p>{{ obligation.task_status }} · {{ obligation.authority_current ? 'Current authority' : 'Authority stale or inactive' }} · observation {{ obligation.observation_ref }}</p></div></div>
        <p v-if="!view.obligations.length">No current obligations.</p>
      </AppPanel>
    </template>
  </main>
</template>

<script setup lang="ts">
import AppPanel from '~/shared/ui/AppPanel.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppAlert from '~/shared/ui/AppAlert.vue'
import { ownerAuthorityRequest, type OwnerGoal, type OwnerMandate, type OwnerAuthorityView, type AuthorityReceipt } from '~/shared/api/owner-goals'

const token = ref(''); const pending = ref(false); const error = ref(''); const view = ref<OwnerAuthorityView | null>(null); const receipt = ref<AuthorityReceipt | null>(null)
const goalKind = ref<OwnerGoal['kind']>('objective'); const goalTitle = ref(''); const goalPurpose = ref(''); const goalConstraints = ref(''); const goalDeadline = ref(''); const goalEditId = ref(''); const goalEditRevision = ref(0); const parentRef = ref('')
const mandateGoalId = ref(''); const origin = ref(''); const pathPrefix = ref('/'); const tools = ref(''); const effects = ref(['read']); const privacy = ref('local_only'); const credentialAlias = ref(''); const credentialField = ref(''); const mandateDeadline = ref(''); const allowInitiative = ref(false); const reporting = ref('exceptions')
const budget = reactive<Record<string, number>>({ model_turns: 20, tokens: 20000, cost_micros: 0, external_calls: 10, children: 2, active_millis: 600000, egress_bytes: 100000, provider_input_bytes: 100000 })
const budgetLimits = [{ key: 'model_turns', label: 'Model turn limit' }, { key: 'tokens', label: 'Token limit' }, { key: 'cost_micros', label: 'Cost limit in micros' }, { key: 'external_calls', label: 'External call limit' }, { key: 'children', label: 'Child task limit' }, { key: 'active_millis', label: 'Active time limit in milliseconds' }, { key: 'egress_bytes', label: 'Network egress byte limit' }, { key: 'provider_input_bytes', label: 'Provider input byte limit' }]
const objectives = computed(() => view.value?.goals.filter(goal => goal.kind === 'objective' && goal.status === 'active') || [])
onBeforeUnmount(() => { token.value = '' })
const load = async () => { pending.value = true; error.value = ''; try { view.value = await ownerAuthorityRequest<OwnerAuthorityView>('/api/v3/owner-authority', token.value) } catch { error.value = 'Owner authority is unavailable. Check authentication.' } finally { pending.value = false } }
const apply = async (path: string, body: unknown) => { pending.value = true; error.value = ''; receipt.value = null; try { receipt.value = await ownerAuthorityRequest<AuthorityReceipt>(path, token.value, body); view.value = await ownerAuthorityRequest<OwnerAuthorityView>('/api/v3/owner-authority', token.value) } catch { error.value = 'Change was rejected. Reload current authority and review its scope or version.' } finally { pending.value = false } }
const resetGoal = () => { goalEditId.value = ''; goalEditRevision.value = 0; parentRef.value = ''; goalTitle.value = ''; goalPurpose.value = ''; goalConstraints.value = ''; goalKind.value = 'objective'; goalDeadline.value = '' }
const editGoal = (goal: OwnerGoal) => { goalEditId.value = goal.goal_id; goalEditRevision.value = goal.revision; goalTitle.value = goal.title; goalPurpose.value = goal.purpose; goalConstraints.value = goal.constraints.join('\n'); goalKind.value = goal.kind; goalDeadline.value = goal.expires_at.slice(0, 16); parentRef.value = '' }
const confirmAssumption = (goal: OwnerGoal) => { editGoal(goal); goalEditId.value = ''; goalEditRevision.value = 0; goalKind.value = 'objective'; parentRef.value = goal.goal_id }
const saveGoal = () => apply('/api/v3/owner-goals', { expected_revision: goalEditRevision.value, parent_ref: parentRef.value || undefined, goal: { goal_id: goalEditId.value, kind: goalKind.value, status: goalKind.value === 'assumption' ? 'proposed' : 'active', title: goalTitle.value, purpose: goalPurpose.value, constraints: goalConstraints.value.split('\n').filter(Boolean), expires_at: new Date(goalDeadline.value).toISOString() } })
const changeGoalStatus = (goal: OwnerGoal, status: 'paused' | 'revoked') => apply('/api/v3/owner-goals', { expected_revision: goal.revision, goal: { goal_id: goal.goal_id, kind: goal.kind, status, title: goal.title, purpose: goal.purpose, constraints: goal.constraints, expires_at: goal.expires_at } })
const changeMandateStatus = (mandate: OwnerMandate, status: 'paused' | 'revoked') => apply('/api/v3/owner-mandates', { expected_revision: mandate.revision, mandate: { ...mandate, status } })
const saveMandate = () => { const goal = objectives.value.find(goal => goal.goal_id === mandateGoalId.value); if (!goal) return; return apply('/api/v3/owner-mandates', { expected_revision: 0, mandate: { goal_id: goal.goal_id, goal_revision: goal.revision, status: 'active', purpose: goal.purpose, tools: tools.value.split(',').map(tool => tool.trim()).filter(Boolean), resource_scopes: [{ origin: origin.value, path_prefix: pathPrefix.value }], effects: effects.value, credential_refs: credentialAlias.value ? [{ alias: credentialAlias.value, field: credentialField.value }] : [], expires_at: new Date(mandateDeadline.value).toISOString(), privacy: privacy.value, budget: { ...budget }, allow_initiative: allowInitiative.value, intervention: ['effect_unknown', 'scope_change', 'budget_exhausted', 'credential_required'], reporting: reporting.value } }) }
</script>

<style scoped>
.page { display: grid; gap: var(--space-4); }
.authority-form { display: grid; gap: var(--space-2); margin-block: var(--space-4); }
label { display: flex; flex-wrap: wrap; gap: var(--space-2); }
input, textarea, select { padding: var(--space-2); border: 1px solid var(--color-border-default); border-radius: var(--radius-md); }
.authority-row { display: flex; flex-wrap: wrap; justify-content: space-between; gap: var(--space-3); padding-block: var(--space-3); border-bottom: 1px solid var(--color-border-default); }
.actions { display: flex; flex-wrap: wrap; align-items: flex-start; gap: var(--space-2); }
</style>
