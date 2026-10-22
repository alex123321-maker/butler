<template><div class="mind-advanced">
  <main class="page">

    <AppPanel title="Owner access">
      <label>Owner control token <input v-model="token" type="password" autocomplete="off" aria-label="Owner control token" /></label>
      <p>The token stays in this page's memory.</p><AppButton :disabled="!token || pending" @click="load">Load converters</AppButton>
      <AppAlert v-if="error" tone="error">{{ error }}</AppAlert><AppAlert v-if="receipt" tone="info">{{ receipt }}</AppAlert>
    </AppPanel>
    <AppPanel v-if="loaded" title="Versions and checked use">
      <p>Development searches existing tools and skills first. Activation requires independent checks, a bounded trial and the exact current <NuxtLink to="/goals">owner converter permission</NuxtLink>. Version changes and rollback use ordinary task actions.</p>
      <article v-for="entry in items" :key="entry.record.digest" class="converter-row">
        <h3>{{ entry.record.converter_id }} {{ entry.record.version }}</h3><p>{{ entry.record.status }} · generation {{ entry.record.generation }}</p>
        <p>Delimited text to JSON. Working credential, file, network and process access: none.</p>
        <p>Input {{ entry.record.permission.input_bytes }} bytes · output {{ entry.record.permission.output_bytes }} bytes · rows {{ entry.record.permission.rows }} · field {{ entry.record.permission.field_bytes }} bytes · runtime {{ entry.record.permission.millis }} ms.</p>
        <p>Trials {{ entry.trials }}/{{ entry.record.permission.trials }} · passed {{ entry.passed_trials }} · checked conversions {{ entry.uses }}.</p>
        <p>{{ entry.checks?.passed ? 'Independent checks passed' : 'Independent checks have not passed' }}<span v-if="entry.checks">: {{ entry.checks.checks.join(', ') }}</span></p>
        <details><summary>Exact version and permission hashes</summary><p>Package {{ entry.record.digest }}</p><p>Need {{ entry.record.need_hash }}</p><p>Runtime {{ entry.record.profile_pin }}</p><p>Input schema {{ entry.record.permission.input_schema_hash }}</p><p>Output schema {{ entry.record.permission.output_schema_hash }}</p></details>
        <p>Task completion still requires its own result verification. Deleted or changed sources cannot be used.</p>
        <div v-for="use in entry.outputs" :key="use.run_id + use.source_ref.source_id" class="actions"><NuxtLink :to="`/tasks/${encodeURIComponent(use.task_id)}`">{{ use.task_id }}</NuxtLink><span>revision {{ use.task_revision }} · converter generation {{ use.generation }}</span><AppButton :disabled="pending" @click="readOutput(use)">Review checked output</AppButton></div>
        <AppButton :disabled="pending || entry.record.status === 'revoked'" @click="revoke(entry)">Permanently revoke {{ entry.record.converter_id }} {{ entry.record.version }}</AppButton>
      </article><p v-if="!items.length">No generated converters.</p>
    </AppPanel>
    <AppPanel v-if="output !== null" title="Checked JSON output"><pre>{{ output }}</pre></AppPanel>
  </main>
</div></template>

<script setup lang="ts">
const mindPanelProps=defineProps<{ownerToken?:string}>()
import AppPanel from '~/shared/ui/AppPanel.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppAlert from '~/shared/ui/AppAlert.vue'
import { converterRequest, type ConverterUse, type ConverterVersion } from '~/shared/api/converters'
import { ownerAuthorityRequest } from '~/shared/api/owner-goals'

const token = ref(mindPanelProps.ownerToken || ''), error = ref(''), receipt = ref(''), loaded = ref(false), pending = ref(false), items = ref<ConverterVersion[]>([]), output = ref<string | null>(null)
watch(token, () => { loaded.value = false; items.value = []; output.value = null; error.value = ''; receipt.value = '' })
onBeforeUnmount(() => { token.value = ''; output.value = null })
async function load() {
  const proof = token.value; pending.value = true; error.value = ''
  try { const response = await converterRequest(proof); if (proof !== token.value) return; if (response.error) throw new Error(); items.value = response.items || []; loaded.value = true }
  catch { error.value = 'Owner access or converter service unavailable.' } finally { pending.value = false }
}
async function revoke(entry: ConverterVersion) {
  const proof = token.value; pending.value = true; error.value = ''; receipt.value = ''
  try { const response = await converterRequest(proof, { operation: 'revoke', digest: entry.record.digest, generation: entry.record.generation }); if (proof !== token.value) return; if (response.error) throw new Error(); output.value = null; receipt.value = 'Exact converter version permanently revoked.'; await load() }
  catch { error.value = 'Decision denied or version changed. Reload current state.' } finally { pending.value = false }
}
async function readOutput(use: ConverterUse) {
  const proof = token.value; pending.value = true; output.value = null; error.value = ''
  try {
    const result = await ownerAuthorityRequest<{ source: { source_id: string; revision: number; access_revision: number; content_hash: string; status: string; body: string } }>(`/api/v4/memory/source?source_id=${encodeURIComponent(use.source_ref.source_id)}`, proof)
    if (proof !== token.value) return
    const source = result.source
    if (!source || source.status !== 'active' || source.source_id !== use.source_ref.source_id || source.revision !== use.source_ref.revision || source.access_revision !== use.source_ref.access_revision || source.content_hash !== use.source_ref.content_hash) throw new Error()
    const bytes = new TextEncoder().encode(source.body); if (bytes.length > 256 * 1024) throw new Error()
    const hash = [...new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))].map(value => value.toString(16).padStart(2, '0')).join('')
    if (hash !== use.sha256 || proof !== token.value) throw new Error()
    output.value = source.body
  } catch { error.value = 'Output deleted, changed or unavailable. No current verified bytes can be displayed.' } finally { pending.value = false }
}
</script>
