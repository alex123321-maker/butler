<template>
  <main class="page">
    <section class="page-header"><div><h2 class="page-title">Memory and sources</h2><p class="page-copy">Source versions, owner decisions and derived search indexes.</p></div></section>
    <AppPanel title="Owner access">
      <label>Owner control token <input v-model="token" type="password" autocomplete="off" aria-label="Owner control token" /></label>
      <p>Personal memory requires authenticated access. The token stays in this page's memory.</p>
      <div class="actions"><AppButton :disabled="!token || pending" @click="load">Load private memory</AppButton><AppButton :disabled="!view || pending" @click="download('/api/v4/memory/export', 'butler-memory.ndjson')">Export all memory</AppButton></div>
      <AppAlert v-if="error" tone="error">{{ error }}</AppAlert><AppAlert v-if="receipt" tone="info">{{ receipt }}</AppAlert>
    </AppPanel>
    <template v-if="view">
      <AppPanel title="Source catalogue">
        <p>Showing at most {{ view.source_list_limit }} sources and {{ view.claim_list_limit }} claims. Export includes all revisions. Source text never grants permissions; <NuxtLink to="/goals">current goals and mandates</NuxtLink> control execution.</p>
        <label>Filter sources <input v-model="filter" aria-label="Filter sources" /></label>
        <div v-for="s in filtered" :key="s.source_id" class="memory-row"><div><strong>{{ s.title || s.external_key }}</strong><p>{{ s.source_kind }} · {{ s.status }} · revision {{ s.revision }} · {{ s.scope_type }}:{{ s.scope_id }}</p><p>{{ s.author || 'Unknown author' }} · event {{ s.event_time || 'unknown' }} · ingested {{ s.extracted_at }}</p><p>{{ s.origin?.kind }} · {{ s.origin?.sensitivity }} · parents {{ s.origin?.parent_refs?.join(', ') }}</p></div><AppButton :disabled="pending" @click="inspect(s.source_id)">Inspect source</AppButton></div>
      </AppPanel>
      <AppPanel v-if="detail" title="Source detail">
        <p>{{ detail.source.source_id }} · revision {{ detail.source.revision }} · {{ detail.source.status }}</p>
        <p>Retained versions {{ detail.revisions.length }} (view limit {{ detail.revision_limit }}). Deleted content is purged; identifiers and deletion evidence remain.</p>
        <details v-for="r in detail.revisions" :key="r.revision"><summary>Revision {{ r.revision }} · {{ r.author }} · event {{ r.event_time || 'unknown' }}</summary><pre>{{ r.body }}</pre><p>Hash {{ r.content_hash }}</p></details>
        <div v-for="a in detail.assets" :key="a.source_revision" class="memory-row"><div><p>{{ a.path || 'Deleted original' }} · {{ a.format }} · revision {{ a.source_revision }} · {{ a.status }} {{ a.error_code }}</p><p>{{ a.parser_version }} · {{ a.method }} · {{ a.uncertain ? 'OCR is uncertain' : 'Structured text' }} · repository {{ a.repository_revision }}</p></div><AppButton :disabled="pending || a.status === 'deleted'" @click="download(`/api/v4/memory/original?source_id=${encodeURIComponent(detail.source.source_id)}&revision=${a.source_revision}`, a.path || 'original')">Download original</AppButton></div>
        <div v-for="f in detail.fragments" :key="f.chunk_id" class="fragment"><strong>{{ f.title }}</strong><pre>{{ f.text }}</pre><p>{{ JSON.stringify(f.locator) }}</p><AppButton :disabled="pending" @click="openCitation(f.chunk_id)">Open fragment context</AppButton></div>
        <template v-if="citation"><h3>Original fragment and neighbours</h3><p>{{ citation.source.title }} · revision {{ citation.source.revision }} · {{ citation.source.author }} · {{ citation.source.event_time || 'Unknown event time' }}</p><pre>{{ citation.fragment.Text }}</pre><pre v-for="n in citation.neighbors || []" :key="n.Key">{{ n.Text }}</pre></template>
        <form v-if="!tombstoned" class="memory-form" @submit.prevent="correct">
          <h3>Correct this source</h3><p>The displayed revision is checked before saving. Old versions remain historical; affected derived records must be rebuilt.</p>
          <label>Source title <input v-model="editTitle" required aria-label="Source title" /></label><label>Source text <textarea v-model="editBody" rows="6" aria-label="Source text" /></label><label>Source author <input v-model="editAuthor" aria-label="Source author" /></label><label>Original event time <input v-model="editTime" type="datetime-local" aria-label="Original event time" /></label>
          <label v-if="detail.source.status === 'quarantined'"><input v-model="reviewLegacy" type="checkbox" /> I reviewed this legacy source; extracted values remain candidates</label><AppButton :disabled="pending || (detail.source.status === 'quarantined' && !reviewLegacy)" type="submit">Save source correction</AppButton>
        </form>
        <form v-if="detail.source.status === 'active'" class="memory-form" @submit.prevent="propose"><h3>Propose a scoped claim</h3><label>Claim key <input v-model="claimKey" required aria-label="Claim key" /></label><label>Claim value as JSON <textarea v-model="claimValue" required aria-label="Claim value as JSON" /></label><label>Claim summary <input v-model="claimSummary" required aria-label="Claim summary" /></label><p>Extraction confidence is not factual truth. Confirmation requires an explicit owner decision.</p><AppButton type="submit" :disabled="pending">Save candidate</AppButton></form>
        <div v-if="!tombstoned" class="actions"><AppButton variant="danger" :disabled="pending" @click="remove('delete')">Delete source and derivatives</AppButton><AppButton :disabled="pending" @click="remove('revoke')">Revoke source access and purge content</AppButton></div>
      </AppPanel>
      <AppPanel title="Scoped claims and history">
        <p>Conflicting candidates remain separate. Confirmation replaces only the displayed current claim in the same scope/key; a concurrent change requires reloading.</p>
        <div v-for="c in view.claims" :key="c.claim_id" class="memory-row"><div><strong>{{ c.key }} · {{ c.status }}</strong><p>{{ c.scope_type }}:{{ c.scope_id }} · {{ c.summary }}</p><pre>{{ JSON.stringify(c.value) }}</pre><p>Source {{ c.source_id }} revision {{ c.source_revision }} · extraction confidence {{ c.extraction_confidence }} · replaces {{ c.supersedes_id || 'none' }}</p></div><div v-if="c.status === 'candidate'" class="actions"><AppButton :disabled="pending" @click="decide(c, false)">Confirm candidate</AppButton><AppButton :disabled="pending" @click="decide(c, true)">Reject candidate</AppButton></div></div>
      </AppPanel>
      <AppPanel title="Upload a selected source">
        <p>Up to 8 MiB and 2,000 fragments. Document content is data. PDF/OCR uses the optional isolated parser; failed files retain a diagnostic and are not indexed.</p>
        <form class="memory-form" @submit.prevent="upload">
          <label>File <input type="file" aria-label="Source file" @change="chooseFile" /></label><label>Stable source key <input v-model="uploadKey" required aria-label="Stable source key" placeholder="repository:logical-file" /></label><label>Expected source revision <input v-model.number="uploadRevision" type="number" min="0" required aria-label="Expected source revision" /></label>
          <label>Format <select v-model="format" aria-label="Source format"><option v-for="f in formats" :key="f" :value="f">{{ f }}</option></select></label><label>Scope <select v-model="scopeType"><option value="user">Owner</option><option value="project">Project</option><option value="task">Task</option><option value="session">Conversation</option></select></label><label>Scope ID <input v-model="scopeId" required aria-label="Source scope ID" /></label><label>File path / title <input v-model="uploadPath" required aria-label="File path" /></label><label>Repository revision <input v-model="repoRevision" aria-label="Repository revision" /></label><label>Author <input v-model="uploadAuthor" aria-label="Upload author" /></label><label>Event time <input v-model="uploadTime" type="datetime-local" aria-label="Upload event time" /></label><AppButton type="submit" :disabled="pending || !file">Parse and save source</AppButton>
        </form>
      </AppPanel>
      <AppPanel title="Task access and owned local indexing">
        <p>{{ view.semantic_enabled ? 'Owned local worker available.' : 'Semantic worker is disabled until its private local service and finite budget policy are configured.' }} Index completion does not confirm claims or activate search automatically.</p>
        <label>Operation <select v-model="operation" aria-label="Memory operation"><option value="grants">Grant exact source access to a current task</option><option v-if="view.semantic_enabled" value="index">Enqueue bounded local index</option><option v-if="view.semantic_enabled" value="index-review">Activate reviewed index</option></select></label>
        <form class="memory-form" @submit.prevent="applyOperation"><label>Versioned operation JSON <textarea v-model="operationJSON" rows="9" required aria-label="Versioned operation JSON" /></label><p>{{ operationHint }}</p><AppButton type="submit" :disabled="pending">Apply versioned memory operation</AppButton></form>
        <div v-for="j in view.index_jobs" :key="j.job_id" class="memory-row"><div><strong>{{ j.job_id }} · {{ j.status }}</strong><p>Generation {{ j.generation_id }} · attempts {{ j.attempts }} · {{ j.error_code }} · deadline {{ j.deadline }}</p><p>Purpose {{ j.purpose }} · original task {{ j.task_id || 'owner finite grant' }} · original run {{ j.original_run_id || 'none' }}</p></div></div>
      </AppPanel>
    </template>
  </main>
</template>

<script setup lang="ts">
import AppPanel from '~/shared/ui/AppPanel.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppAlert from '~/shared/ui/AppAlert.vue'
import { ownerAuthorityRequest } from '~/shared/api/owner-goals'
interface Source { source_id: string; external_key: string; source_kind: string; scope_type: string; scope_id: string; revision: number; status: string; title: string; body: string; author: string; event_time: string | null; extracted_at: string; origin?: { kind: string; sensitivity: string; parent_refs?: string[] } }
interface Claim { claim_id: string; key: string; scope_type: string; scope_id: string; summary: string; value: unknown; status: string; source_id: string; source_revision: number; extraction_confidence: number; supersedes_id: string }
interface Asset { source_revision: number; path: string; format: string; repository_revision: string; parser_version: string; method: string; status: string; uncertain: boolean; error_code: string }
interface Fragment { Key: string; Text: string }
interface Inspection { source: Source; revisions: { revision: number; body: string; author: string; event_time: string; content_hash: string }[]; fragments: { chunk_id: number; title: string; text: string; locator: unknown }[]; assets: Asset[]; revision_limit: number }
interface View { sources: Source[]; claims: Claim[]; source_list_limit: number; claim_list_limit: number; semantic_enabled: boolean; index_jobs: { job_id: string; status: string; generation_id: string; attempts: number; error_code: string; purpose: string; deadline: string; task_id: string; original_run_id: string }[] }
const token = ref(''); const pending = ref(false); const error = ref(''); const receipt = ref(''); const view = ref<View | null>(null); const detail = ref<Inspection | null>(null); const citation = ref<{ source: Source; fragment: Fragment; neighbors: Fragment[] } | null>(null)
const filter = ref(''); const editTitle = ref(''); const editBody = ref(''); const editAuthor = ref(''); const editTime = ref(''); const reviewLegacy = ref(false); const claimKey = ref(''); const claimValue = ref('""'); const claimSummary = ref('')
const file = ref<File | null>(null); const uploadKey = ref(''); const uploadRevision = ref(0); const format = ref('text'); const formats = ['text', 'markdown', 'go', 'csv', 'tsv', 'messages', 'docx', 'pdf']; const scopeType = ref('user'); const scopeId = ref(''); const uploadPath = ref(''); const repoRevision = ref(''); const uploadAuthor = ref(''); const uploadTime = ref(''); const operation = ref('grants'); const operationJSON = ref('')
const operationHint = computed(() => operation.value === 'grants' ? 'Fields: source_id, task_id, task_revision, purpose. The task and source must belong to this owner.' : operation.value === 'index' ? 'Fields: job {job_id, kind:index, purpose, task_id, original_run_id, sources [{source_id, revision, access_revision, content_hash}], allowance {model_turns,tokens,active_millis,provider_input_bytes}, deadline}, scope_type, scope_id, max_chunks. A completed session lease cannot authorize this worker.' : 'Fields: generation_id, expected_previous, proof {artifact_ref,artifact_sha256,corpus_version,cases,baseline_top1,candidate_top1}. Reviewed quality evidence is required.')
const filtered = computed(() => view.value?.sources.filter(s => `${s.title} ${s.external_key} ${s.scope_id}`.toLowerCase().includes(filter.value.toLowerCase())) || [])
const tombstoned = computed(() => detail.value?.source.status === 'deleted' || detail.value?.source.status === 'revoked')
interface OwnerProof { epoch: number; token: string }
let proofEpoch = 0
let selectionEpoch = 0
let queuedInspection: string | null = null
let mounted = true
const proofCurrent = (proof: OwnerProof) => mounted && proof.epoch === proofEpoch && !!proof.token && proof.token === token.value
const requireProof = (proof: OwnerProof) => { if (!proofCurrent(proof)) throw new Error('owner proof changed') }
const clearPrivate = () => {
  selectionEpoch++; queuedInspection = null
  view.value = null; detail.value = null; citation.value = null; file.value = null
  error.value = ''; receipt.value = ''; pending.value = false; filter.value = ''
  editTitle.value = ''; editBody.value = ''; editAuthor.value = ''; editTime.value = ''; reviewLegacy.value = false
  claimKey.value = ''; claimValue.value = '""'; claimSummary.value = ''
  uploadKey.value = ''; uploadRevision.value = 0; uploadPath.value = ''; repoRevision.value = ''; uploadAuthor.value = ''; uploadTime.value = ''
  scopeType.value = 'user'; scopeId.value = ''; format.value = 'text'; operation.value = 'grants'; operationJSON.value = ''
}
watch(token, () => { proofEpoch++; clearPrivate() }, { flush: 'sync' })
onBeforeUnmount(() => { mounted = false; proofEpoch++; token.value = ''; clearPrivate() })
const request = async <T,>(proof: OwnerProof, path: string, body?: unknown): Promise<T> => {
  requireProof(proof)
  const result = await ownerAuthorityRequest<T>(path, proof.token, body)
  requireProof(proof)
  return result
}
const localTime = (v: string | null) => v ? new Date(new Date(v).getTime() - new Date(v).getTimezoneOffset() * 60000).toISOString().slice(0, 16) : ''
const eventTime = (v: string) => v ? new Date(v).toISOString() : null
const loadView = async (proof: OwnerProof) => { view.value = await request<View>(proof, '/api/v4/memory') }
const run = async (fn: (proof: OwnerProof) => Promise<void>) => {
  if (pending.value || !token.value || !mounted) return
  const proof = { epoch: proofEpoch, token: token.value }
  pending.value = true; error.value = ''; receipt.value = ''
  try { await fn(proof) } catch {
    if (proofCurrent(proof)) error.value = 'Memory request rejected. Check owner access, current revision, scope or deletion anchor; reload before retrying.'
  } finally { if (proofCurrent(proof)) { pending.value = false; flushInspection() } }
}
const route = useRoute()
const selectedSource = computed(() => typeof route.query.source_id === 'string' && route.query.source_id.length <= 256 ? route.query.source_id : '')
const clearDetail = () => { detail.value = null; citation.value = null; editTitle.value = ''; editBody.value = ''; editAuthor.value = ''; editTime.value = ''; reviewLegacy.value = false }
const load = () => run(async proof => { await loadView(proof); const id = queuedInspection || selectedSource.value; queuedInspection = null; if (id) await loadDetail(proof, id, selectionEpoch) })
const loadDetail = async (proof: OwnerProof, id: string, epoch = selectionEpoch) => {
  requireProof(proof)
  if (epoch !== selectionEpoch) return
  clearDetail()
  const loaded = await request<Inspection>(proof, `/api/v4/memory/source?source_id=${encodeURIComponent(id)}`)
  if (epoch !== selectionEpoch) return
  if (loaded.source.source_id !== id) throw new Error('source selection differs')
  detail.value = loaded
  const s = loaded.source
  editTitle.value = s.title; editBody.value = s.body; editAuthor.value = s.author; editTime.value = localTime(s.event_time); reviewLegacy.value = false
}
const flushInspection = () => {
  if (pending.value || !view.value || !queuedInspection) return
  const id = queuedInspection; queuedInspection = null
  const epoch = selectionEpoch
  return run(proof => loadDetail(proof, id, epoch))
}
const inspect = (id: string) => { selectionEpoch++; clearDetail(); queuedInspection = id; return flushInspection() }
watch(selectedSource, id => { selectionEpoch++; clearDetail(); queuedInspection = id || null; flushInspection() }, { flush: 'sync' })
const mutate = async (proof: OwnerProof, path: string, body: unknown) => {
  const sourceID = detail.value?.source.source_id
  const epoch = selectionEpoch
  await request(proof, path, body)
  if (epoch !== selectionEpoch) return
  await loadView(proof)
  if (epoch !== selectionEpoch) return
  if (sourceID) await loadDetail(proof, sourceID, epoch)
  if (epoch !== selectionEpoch) return
  requireProof(proof)
  receipt.value = 'Saved with the current source revision. Historical evidence remains separate from current authority.'
}
const correct = () => run(proof => mutate(proof, '/api/v4/memory/correct', { source_id: detail.value?.source.source_id, expected_revision: detail.value?.source.revision, title: editTitle.value, body: editBody.value, author: editAuthor.value, event_time: eventTime(editTime.value), review_legacy: reviewLegacy.value }))
const propose = () => run(proof => mutate(proof, '/api/v4/memory/claims', { source_id: detail.value?.source.source_id, source_revision: detail.value?.source.revision, key: claimKey.value, value: JSON.parse(claimValue.value), summary: claimSummary.value, extraction_confidence: 1 }))
const decide = (c: Claim, reject: boolean) => run(proof => { const a = view.value?.claims.find(a => a.status === 'confirmed' && a.scope_type === c.scope_type && a.scope_id === c.scope_id && a.key === c.key); return mutate(proof, '/api/v4/memory/decisions', { claim_id: c.claim_id, expected_active_id: a?.claim_id || '', reject }) })
const remove = (action: string) => run(proof => mutate(proof, '/api/v4/memory/delete', { source_id: detail.value?.source.source_id, expected_revision: detail.value?.source.revision, action }))
const openCitation = (id: number) => run(async proof => { const epoch = selectionEpoch; const loaded = await request<{ source: Source; fragment: Fragment; neighbors: Fragment[] }>(proof, `/api/v4/memory/citation?chunk_id=${id}`); if (epoch === selectionEpoch) citation.value = loaded })
const chooseFile = (e: Event) => { file.value = (e.target as HTMLInputElement).files?.[0] || null; uploadPath.value = file.value?.name || '' }
const upload = () => run(async proof => { if (!file.value || file.value.size > 8 * 1024 * 1024) throw new Error('file limit'); const bytes = new Uint8Array(await file.value.arrayBuffer()); requireProof(proof); let binary = ''; for (let i = 0; i < bytes.length; i += 16384) binary += String.fromCharCode(...bytes.subarray(i, i + 16384)); const r = await request<{ source: Source; status: string; error_code: string; uncertain: boolean }>(proof, '/api/v4/memory/ingest', { external_key: uploadKey.value, expected_revision: uploadRevision.value, scope_type: scopeType.value, scope_id: scopeId.value, format: format.value, path: uploadPath.value, repository_revision: repoRevision.value, author: uploadAuthor.value, event_time: eventTime(uploadTime.value), original_bytes: btoa(binary) }); await loadView(proof); await loadDetail(proof, r.source.source_id); receipt.value = `Source revision ${r.source.revision}: ${r.status} ${r.error_code || ''}${r.uncertain ? ' · OCR is uncertain' : ''}` })
const applyOperation = () => run(proof => mutate(proof, `/api/v4/memory/${operation.value}`, JSON.parse(operationJSON.value)))
const download = (path: string, filename: string) => run(async proof => { const config = useRuntimeConfig(); const r = await fetch(`${String(config.public.apiBase || '').replace(/\/$/, '')}${path}`, { headers: { Authorization: `Bearer ${proof.token}` }, cache: 'no-store' }); requireProof(proof); if (!r.ok) throw new Error('access'); const blob = await r.blob(); requireProof(proof); const url = URL.createObjectURL(blob); const a = document.createElement('a'); a.href = url; a.download = filename; a.click(); URL.revokeObjectURL(url) })
</script>

<style scoped>
.memory-row { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-4); padding: var(--space-4) 0; border-bottom: 1px solid var(--color-border-default); }
.memory-form { display: grid; gap: var(--space-3); margin: var(--space-4) 0; }
.actions { display: flex; gap: var(--space-2); flex-wrap: wrap; }
label { display: grid; gap: var(--space-2); }
input, select, textarea { max-width: 100%; padding: var(--space-2); background: var(--color-bg-surfaceMuted); color: var(--color-text-primary); border: 1px solid var(--color-border-default); border-radius: var(--radius-sm); }
pre { white-space: pre-wrap; overflow-wrap: anywhere; max-height: 24rem; overflow: auto; }
.fragment { padding: var(--space-3); border: 1px solid var(--color-border-default); margin: var(--space-3) 0; }
@media (max-width: 860px) { .memory-row { display: grid; } }
</style>
