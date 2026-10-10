<template>
  <main class="page">
    <section class="page-header"><div><h2 class="page-title">Reviewed extensions</h2><p class="page-copy">Install local manifests, review their provenance and grant bounded access.</p></div></section>
    <AppPanel title="Owner access">
      <label>Owner control token <input v-model="token" type="password" autocomplete="off" aria-label="Owner control token" /></label>
      <p>The token stays in this page's memory. Installing a package does not enable it or authorize any task.</p>
      <AppButton :disabled="!token || pending" @click="load">Load extensions</AppButton>
      <AppAlert v-if="error" tone="error">{{ error }}</AppAlert>
      <AppAlert v-if="receipt" tone="info">{{ receipt }}</AppAlert>
    </AppPanel>
    <template v-if="loaded">
      <AppPanel title="Install a local manifest">
        <label>Reviewed manifest JSON <textarea v-model="manifestJSON" rows="10" aria-label="Reviewed manifest JSON" /></label>
        <p>No URL is fetched and no package code is run. Review the exact source, dependencies, schema, resource paths and runtime profile.</p>
        <AppButton :disabled="pending || !manifestJSON" @click="stage">Install manifest</AppButton>
      </AppPanel>
      <AppPanel title="Installed capabilities">
        <div v-for="item in items" :key="item.state.extension_id" class="extension-row">
          <div><strong>{{ item.state.extension_id }}</strong><p>{{ item.manifest.version }} · {{ item.state.status }} · {{ item.state.health }} · {{ item.state.available ? 'Available' : 'Unavailable' }}</p><p>{{ item.state.health_reason }} · generation {{ item.state.generation }}</p><p>Reviewed: {{ item.state.reviewed ? 'yes' : 'no' }} · health expires {{ item.state.healthy_until || 'not validated' }}</p></div>
          <AppButton :disabled="pending" @click="select(item)">Manage {{ item.state.extension_id }}</AppButton>
        </div>
        <p v-if="!items.length">No reviewed extensions installed.</p>
      </AppPanel>
      <AppPanel v-if="selected" :title="`Manage ${selected.state.extension_id}`">
        <p>Package text is external data: {{ selected.manifest.description }}</p>
        <p>Source {{ selected.manifest.source.uri }} · revision {{ selected.manifest.source.revision }}</p>
        <p class="digest">Manifest SHA-256 {{ selected.state.manifest_hash }}</p>
        <p>Runtime profile {{ selected.manifest.runtime.profile }} · {{ selected.manifest.runtime.abi }}</p>
        <ul><li v-for="dependency in selected.manifest.dependencies" :key="dependency.id">{{ dependency.id }} {{ dependency.version }} · {{ dependency.source }} · {{ dependency.sha256 }}</li></ul>
        <label>Installed version <select v-model="versionHash" aria-label="Installed version" @change="selectVersion"><option v-for="version in versions" :key="version.manifest_hash" :value="version.manifest_hash">{{ version.manifest.version }} · {{ version.manifest_hash.slice(0, 12) }}</option></select></label>
        <AppButton :disabled="pending || !versionHash" @click="review">Record review of selected version</AppButton>
        <form class="extension-form" @submit.prevent="configure">
          <h3>Configuration and capability subset</h3>
          <label v-for="setting in editingManifest.configuration" :key="setting.name">{{ setting.name }} {{ setting.required ? '(required for readiness)' : '(optional)' }} <input v-model="configuration[setting.name]" :type="setting.type === 'public_origin' ? 'url' : 'text'" :aria-label="`Setting ${setting.name}`" /></label>
          <label v-for="credential in editingManifest.credentials" :key="credential.input">Credential alias for {{ credential.input }} / {{ credential.field }} <input v-model="aliases[credential.input]" :aria-label="`Credential alias ${credential.input}`" placeholder="Metadata alias only" /></label>
          <label v-for="operation in editingManifest.operations" :key="operation.name"><input v-model="chosenTools" type="checkbox" :value="operation.name" :aria-label="operation.name" /> {{ operation.name }} · {{ operation.method }} · {{ operation.effect }} · {{ operation.receipt?.protocol || 'no recipient receipt' }}</label>
          <p>Saving or rolling back disables this capability and changes its generation. Fresh readiness and task permissions are required.</p>
          <div class="actions"><AppButton type="submit" :disabled="pending || !chosenTools.length">Save configuration and version</AppButton><AppButton type="button" :disabled="pending || !chosenTools.length || versionHash === selected.state.manifest_hash" @click="configure('rollback')">Roll back to selected version</AppButton></div>
        </form>
        <div class="actions"><AppButton :disabled="pending || !selected.state.reviewed" @click="action('probe')">Validate readiness</AppButton><AppButton :disabled="pending || selected.state.health !== 'healthy' || !selected.state.reviewed || selected.state.status === 'enabled'" @click="action('enable')">Enable capability</AppButton><AppButton :disabled="pending || selected.state.status !== 'enabled'" variant="danger" @click="action('disable')">Disable capability</AppButton></div>
        <form class="extension-form" @submit.prevent="grant">
          <h3>Exact task attachment</h3><p>Specify current task/constraint versions, selected tools, canonical resource scopes, credential alias fields, finite expiry and the existing permission revision for an amendment. Goal-backed tasks use the exact reviewed extension entry in their owner mandate.</p>
          <label>Task permission JSON <textarea v-model="accessJSON" rows="10" aria-label="Task permission JSON" /></label>
          <AppButton type="submit" :disabled="pending || !selected.state.available || !accessJSON">Grant task access</AppButton>
        </form>
      </AppPanel>
    </template>
  </main>
</template>

<script setup lang="ts">
import AppPanel from '~/shared/ui/AppPanel.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppAlert from '~/shared/ui/AppAlert.vue'
import { extensionRequest, type ExtensionItem, type ExtensionManifest } from '~/shared/api/extensions'

const token = ref(''), manifestJSON = ref(''), accessJSON = ref(''), error = ref(''), receipt = ref('')
const pending = ref(false), loaded = ref(false), items = ref<ExtensionItem[]>([]), selected = ref<ExtensionItem>()
const versions = ref<{ manifest: ExtensionManifest; manifest_hash: string }[]>([]), versionHash = ref('')
const configuration = ref<Record<string, string>>({}), aliases = ref<Record<string, string>>({}), chosenTools = ref<string[]>([])
const editingManifest = computed(() => versions.value.find(version => version.manifest_hash === versionHash.value)?.manifest || selected.value!.manifest)
async function load() {
  pending.value = true; error.value = ''
  try { const response = await extensionRequest(token.value); items.value = response.items || []; loaded.value = true; if (selected.value) selected.value = items.value.find(item => item.state.extension_id === selected.value?.state.extension_id) }
  catch { error.value = 'Owner access or extension service unavailable.' } finally { pending.value = false }
}
async function select(item: ExtensionItem) {
  selected.value = item; versionHash.value = item.state.manifest_hash; configuration.value = { ...item.state.configuration }; chosenTools.value = [...(item.state.allowed_tools || [])]; aliases.value = Object.fromEntries(Object.entries(item.state.credential_refs || {}).map(([key, value]) => [key, value.alias])); accessJSON.value = ''
  try { const response = await extensionRequest(token.value, { operation: 'versions', extension_id: item.state.extension_id }); versions.value = response.versions || [] } catch { error.value = 'Version history unavailable.' }
}
function selectVersion() { configuration.value = {}; aliases.value = {}; chosenTools.value = [] }
async function control(request: Record<string, unknown>, message: string) {
  pending.value = true; error.value = ''; receipt.value = ''
  try { const response = await extensionRequest(token.value, request); if (response.error) throw new Error(); receipt.value = message; await load() }
  catch { error.value = 'Action denied or state changed. Reload and review current state.' } finally { pending.value = false }
}
async function stage() {
  // Keep the exact reviewed bytes, including whitespace. The Broker parses
  // duplicate/unknown members; browser JSON.parse must not erase ambiguity.
  await control({ operation: 'stage', manifest_text: manifestJSON.value }, 'Manifest installed for review; no execution permission granted.')
}
async function review() { if (selected.value) await control({ operation: 'review', extension_id: selected.value.state.extension_id, manifest_hash: versionHash.value }, 'Review recorded for the exact version and configured runtime dependency.') }
async function configure(operation: 'configure' | 'rollback' | Event = 'configure') {
  if (!selected.value) return
  const refs = Object.fromEntries((editingManifest.value.credentials || []).filter(input => aliases.value[input.input]).map(input => [input.input, { alias: aliases.value[input.input], field: input.field }]))
  await control({ operation: operation === 'rollback' ? 'rollback' : 'configure', extension_id: selected.value.state.extension_id, manifest_hash: versionHash.value, generation: selected.value.state.generation, configuration: configuration.value, credential_refs: refs, tools: chosenTools.value }, 'Configuration saved disabled. Validate readiness and grant fresh task access.')
}
async function action(operation: string) { if (selected.value) await control({ operation, extension_id: selected.value.state.extension_id, generation: selected.value.state.generation }, operation === 'probe' ? 'Readiness validation recorded; task access remains separate.' : 'Capability state changed. Historical tasks and effects remain recorded.') }
async function grant() {
  if (!selected.value) return
  try { await control({ operation: 'grant_task', extension_id: selected.value.state.extension_id, access: { ...JSON.parse(accessJSON.value), manifest_hash: selected.value.state.manifest_hash, generation: selected.value.state.generation } }, 'Bounded task permission saved for the current manifest and generation.') } catch { error.value = 'Task permission must be valid JSON.' }
}
</script>

<style scoped>
.extension-row { display: flex; justify-content: space-between; gap: var(--space-4); padding-block: var(--space-3); border-bottom: 1px solid var(--color-border-default); }
.extension-form { display: grid; gap: var(--space-3); margin-block: var(--space-5); }
label { display: grid; gap: var(--space-2); }
input, textarea, select { background: var(--color-bg-surface); color: var(--color-text-primary); border: 1px solid var(--color-border-default); border-radius: var(--radius-sm); padding: var(--space-2); }
.actions { display: flex; flex-wrap: wrap; gap: var(--space-2); margin-block: var(--space-3); }
.digest { overflow-wrap: anywhere; }
</style>
