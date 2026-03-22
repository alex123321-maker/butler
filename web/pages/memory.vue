<template>
  <main class="page">
    <section class="page-header">
      <div class="page-header__content">
        <p class="page-kicker">Memory</p>
        <h2 class="page-title">Memory</h2>
        <p class="page-copy">
          Inspect live task context and long-term memory without manually digging up internal scope ids first.
        </p>
      </div>
      <div class="page-header__actions">
        <AppButton variant="secondary" :disabled="pending" @click="showManualScope = !showManualScope">
          {{ showManualScope ? 'Hide manual scope' : 'Manual scope' }}
        </AppButton>
        <AppButton variant="primary" :disabled="pending || !hasScope" @click="refresh">Refresh</AppButton>
      </div>
    </section>

    <section class="page-section">
      <div class="context-header">
        <div>
          <h3 class="section-title">Current context</h3>
          <p class="section-copy">
            Working memory lives on the session scope, so this page now defaults to the most recent session when possible.
          </p>
        </div>
        <div class="pill-row" v-if="routeSourceTask || hasScope">
          <span v-if="routeSourceTask" class="pill">From {{ routeSourceTask }}</span>
          <span v-if="hasScope" class="pill">{{ selectedScopeLabel }}</span>
        </div>
      </div>

      <div v-if="recentSessions.length" class="recent-contexts">
        <button
          v-for="session in recentSessions"
          :key="session.session_key"
          type="button"
          class="context-card"
          :class="{ 'context-card--active': filters.scopeType === 'session' && filters.scopeID === session.session_key }"
          @click="selectSession(session.session_key)"
        >
          <p class="context-card__title">{{ session.channel }}</p>
          <p class="context-card__meta">{{ session.session_key }}</p>
          <p class="context-card__meta">Updated {{ formatDateTime(session.updated_at) }}</p>
        </button>
      </div>

      <p v-else-if="sessionsPending" class="placeholder-text">Loading recent contexts…</p>
      <p v-else-if="!hasScope" class="placeholder-text">No recent session context is available yet.</p>

      <div v-if="showManualScope" class="manual-scope">
        <label>
          Scope type
          <AppSelect v-model="filters.scopeType">
            <option value="session">Session</option>
            <option value="user">User</option>
            <option value="global">Global</option>
          </AppSelect>
        </label>
        <label>
          Scope id
          <AppInput v-model="filters.scopeID" type="text" placeholder="telegram:chat:123 or user id" />
        </label>
        <div class="manual-scope__actions">
          <AppButton variant="primary" :disabled="pending || !hasScope" @click="refresh">Load scope</AppButton>
        </div>
      </div>
    </section>

    <section v-if="hasScope" class="page-section">
      <div class="memory-filters">
        <label>
          Confirmation
          <AppSelect v-model="filters.confirmationState">
            <option value="all">All</option>
            <option value="pending">Pending</option>
            <option value="confirmed">Confirmed</option>
            <option value="rejected">Rejected</option>
            <option value="auto_confirmed">Auto-confirmed</option>
          </AppSelect>
        </label>
        <label>
          Status
          <AppSelect v-model="filters.effectiveStatus">
            <option value="all">All</option>
            <option value="active">Active</option>
            <option value="inactive">Inactive</option>
            <option value="suppressed">Suppressed</option>
            <option value="expired">Expired</option>
            <option value="deleted">Deleted</option>
          </AppSelect>
        </label>
        <label class="memory-toggle">
          <input v-model="filters.showSuppressed" type="checkbox" />
          <span>Show suppressed entries</span>
        </label>
      </div>
    </section>

    <AppAlert v-if="error" tone="error">{{ error }}</AppAlert>

    <AppEmptyState
      v-if="!hasScope && !sessionsPending"
      title="No memory context selected"
      description="Pick a recent session above or open memory from a specific task."
    />
    <p v-else-if="pending" class="placeholder-text">Loading memory…</p>

    <template v-else>
      <section class="memory-layout">
        <section class="memory-card memory-card--primary">
          <div class="memory-card__header">
            <div>
              <h3>Working memory</h3>
              <p class="memory-card__copy">Live context Butler is actively using for the selected session.</p>
            </div>
            <span class="pill" v-if="memoryData?.working?.status">{{ memoryData.working.status }}</span>
          </div>

          <div v-if="memoryData?.working" class="memory-fields">
            <MemoryField label="Goal" :value="memoryData.working.goal" />
            <MemoryField label="Status" :value="memoryData.working.status" />
            <MemoryField label="Source" :value="`${memoryData.working.source_type}:${memoryData.working.source_id}`" />
            <MemoryField label="Pending steps" :value="memoryData.working.pending_steps_json" block />
            <MemoryField label="Entities" :value="memoryData.working.entities_json" block />
            <MemoryField label="Provenance" :value="memoryData.working.provenance" block />
          </div>
          <p v-else class="placeholder-text">
            {{ filters.scopeType === 'session' ? 'No working memory is stored for this session yet.' : 'Working memory is only available on session context.' }}
          </p>
        </section>

        <section class="memory-card">
          <div class="memory-card__header">
            <div>
              <h3>Profile memory</h3>
              <p class="memory-card__copy">Long-lived facts and preferences Butler should remember about you.</p>
            </div>
            <span class="pill">{{ filteredProfile.length }}</span>
          </div>
          <div v-if="filteredProfile.length" class="memory-list">
            <article v-for="item in filteredProfile" :key="item.id" class="memory-item" :class="{ 'memory-item--suppressed': item.suppressed }">
              <header>
                <strong>{{ item.key }}</strong>
                <div class="badges">
                  <AppBadge :tone="getConfirmationTone(item.confirmation_state)">{{ item.confirmation_state }}</AppBadge>
                  <AppBadge :tone="getEffectiveTone(item.effective_status)">{{ item.effective_status }}</AppBadge>
                </div>
              </header>
              <p class="summary">{{ item.summary }}</p>
              <MemoryField label="Value" :value="item.value_json" block />
              <MemoryField label="Provenance" :value="item.provenance" block />
              <MemoryField v-if="item.edited_by" label="Edited by" :value="`${item.edited_by} at ${item.edited_at}`" />
              <MemoryField label="Links" :value="formatLinks(item.links)" block />

              <div class="memory-actions">
                <template v-if="item.capabilities.confirmable && item.confirmation_state === 'pending'">
                  <AppButton variant="primary" :disabled="actionPending" @click="handleConfirm('profile', item.id)">Confirm</AppButton>
                  <AppButton variant="danger" :disabled="actionPending" @click="handleReject('profile', item.id)">Reject</AppButton>
                </template>
                <template v-if="item.capabilities.suppressible">
                  <AppButton v-if="!item.suppressed" variant="secondary" :disabled="actionPending" @click="handleSuppress('profile', item.id)">Suppress</AppButton>
                  <AppButton v-else variant="secondary" :disabled="actionPending" @click="handleUnsuppress('profile', item.id)">Unsuppress</AppButton>
                </template>
                <AppButton v-if="item.capabilities.editable" variant="secondary" :disabled="actionPending" @click="openEditDialog(item)">Edit</AppButton>
                <AppButton v-if="item.capabilities.deletable && item.effective_status !== 'deleted'" variant="danger" :disabled="actionPending" @click="handleDelete('profile', item.id)">Delete</AppButton>
              </div>
            </article>
          </div>
          <p v-else class="placeholder-text">No profile memory entries for this scope.</p>
        </section>

        <section class="memory-card">
          <div class="memory-card__header">
            <div>
              <h3>Episodes</h3>
              <p class="memory-card__copy">Important moments Butler extracted from past work and conversations.</p>
            </div>
            <span class="pill">{{ filteredEpisodic.length }}</span>
          </div>
          <div v-if="filteredEpisodic.length" class="memory-list">
            <article v-for="item in filteredEpisodic" :key="item.id" class="memory-item" :class="{ 'memory-item--suppressed': item.suppressed }">
              <header>
                <strong>{{ item.summary }}</strong>
                <div class="badges">
                  <AppBadge :tone="getConfirmationTone(item.confirmation_state)">{{ item.confirmation_state }}</AppBadge>
                  <AppBadge :tone="getEffectiveTone(item.effective_status)">{{ item.effective_status }}</AppBadge>
                </div>
              </header>
              <p class="summary">{{ item.content }}</p>
              <MemoryField label="Tags" :value="item.tags_json" block />
              <MemoryField label="Provenance" :value="item.provenance" block />
              <MemoryField v-if="item.edited_by" label="Edited by" :value="`${item.edited_by} at ${item.edited_at}`" />
              <MemoryField label="Links" :value="formatLinks(item.links)" block />

              <div class="memory-actions">
                <template v-if="item.capabilities.confirmable && item.confirmation_state === 'pending'">
                  <AppButton variant="primary" :disabled="actionPending" @click="handleConfirm('episodic', item.id)">Confirm</AppButton>
                  <AppButton variant="danger" :disabled="actionPending" @click="handleReject('episodic', item.id)">Reject</AppButton>
                </template>
                <template v-if="item.capabilities.suppressible">
                  <AppButton v-if="!item.suppressed" variant="secondary" :disabled="actionPending" @click="handleSuppress('episodic', item.id)">Suppress</AppButton>
                  <AppButton v-else variant="secondary" :disabled="actionPending" @click="handleUnsuppress('episodic', item.id)">Unsuppress</AppButton>
                </template>
                <AppButton v-if="item.capabilities.deletable && item.effective_status !== 'deleted'" variant="danger" :disabled="actionPending" @click="handleDelete('episodic', item.id)">Delete</AppButton>
              </div>
            </article>
          </div>
          <p v-else class="placeholder-text">No episodic memory entries for this scope.</p>
        </section>

        <section class="memory-card">
          <div class="memory-card__header">
            <div>
              <h3>Reference chunks</h3>
              <p class="memory-card__copy">Chunked long-form knowledge Butler can reuse later.</p>
            </div>
            <span class="pill">{{ filteredChunks.length }}</span>
          </div>
          <div v-if="filteredChunks.length" class="memory-list">
            <article v-for="item in filteredChunks" :key="item.id" class="memory-item" :class="{ 'memory-item--suppressed': item.suppressed }">
              <header>
                <strong>{{ item.title }}</strong>
                <div class="badges">
                  <AppBadge :tone="getEffectiveTone(item.effective_status)">{{ item.effective_status }}</AppBadge>
                </div>
              </header>
              <p class="summary">{{ item.summary }}</p>
              <MemoryField label="Content" :value="item.content" block />
              <MemoryField label="Tags" :value="item.tags_json" block />
              <MemoryField label="Provenance" :value="item.provenance" block />
              <MemoryField label="Links" :value="formatLinks(item.links)" block />

              <div class="memory-actions">
                <template v-if="item.capabilities.suppressible">
                  <AppButton v-if="!item.suppressed" variant="secondary" :disabled="actionPending" @click="handleSuppress('chunk', item.id)">Suppress</AppButton>
                  <AppButton v-else variant="secondary" :disabled="actionPending" @click="handleUnsuppress('chunk', item.id)">Unsuppress</AppButton>
                </template>
                <AppButton v-if="item.capabilities.deletable" variant="danger" :disabled="actionPending" @click="handleDelete('chunk', item.id)">Delete</AppButton>
              </div>
            </article>
          </div>
          <p v-else class="placeholder-text">No chunk memory entries for this scope.</p>
        </section>
      </section>
    </template>

    <AppDialog :model-value="editDialogOpen" @update:model-value="onDialogToggle">
      <template #title>
        <h3 class="dialog-title">Edit profile memory</h3>
      </template>
      <div class="edit-form">
        <label>
          Key
          <AppInput :model-value="editingItem?.key" disabled />
        </label>
        <label>
          Summary
          <AppInput v-model="editForm.summary" type="text" placeholder="Summary" />
        </label>
        <label>
          Value (JSON)
          <textarea v-model="editForm.value_json" class="json-textarea" rows="6" placeholder='{"key": "value"}'></textarea>
        </label>
      </div>
      <template #footer>
        <div class="dialog-actions">
          <AppButton variant="secondary" @click="closeEditDialog">Cancel</AppButton>
          <AppButton variant="primary" :disabled="actionPending" @click="submitEdit">Save</AppButton>
        </div>
      </template>
    </AppDialog>
  </main>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useSessions } from '~/composables/useApi'
import { formatDateTime } from '~/entities/task/presentation'
import { useMemoryStore } from '~/shared/model/stores/memory'
import type { MemoryLinkRecord, MemoryType, ConfirmationState, EffectiveStatus, ProfileMemoryRecord } from '~/entities/memory/api'
import AppAlert from '~/shared/ui/AppAlert.vue'
import AppBadge from '~/shared/ui/AppBadge.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppDialog from '~/shared/ui/AppDialog.vue'
import AppEmptyState from '~/shared/ui/AppEmptyState.vue'
import AppInput from '~/shared/ui/AppInput.vue'
import AppSelect from '~/shared/ui/AppSelect.vue'

useHead({ title: 'Memory - Butler' })

const memoryStore = useMemoryStore()
const { records: memoryData, pending, actionPending, error, filters, filteredProfile, filteredEpisodic, filteredChunks } = storeToRefs(memoryStore)
const route = useRoute()
const { data: sessionsData, pending: sessionsPending } = useSessions()

const hasScope = computed(() => filters.value.scopeID.trim().length > 0)
const showManualScope = ref(false)
const initializedScope = ref(false)
const routeSourceTask = computed(() => String(route.query.sourceTask || ''))

const recentSessions = computed(() => {
  return [...(sessionsData.value ?? [])]
    .sort((left, right) => new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime())
    .slice(0, 6)
})

const selectedScopeLabel = computed(() => {
  if (!hasScope.value) {
    return 'No scope selected'
  }
  return `${filters.value.scopeType}: ${filters.value.scopeID}`
})

const routeScopeType = computed(() => {
  const value = route.query.scopeType ?? route.query.scope_type
  return typeof value === 'string' ? value : ''
})

const routeScopeID = computed(() => {
  const value = route.query.scopeId ?? route.query.scope_id
  return typeof value === 'string' ? value : ''
})

watch(
  [routeScopeType, routeScopeID, recentSessions],
  async ([scopeType, scopeID, sessions]) => {
    if (scopeType && scopeID) {
      initializedScope.value = true
      await memoryStore.load({
        scopeType: scopeType as 'session' | 'user' | 'global',
        scopeID,
      })
      return
    }

    if (initializedScope.value || hasScope.value || !sessions.length) {
      return
    }

    initializedScope.value = true
    await memoryStore.load({
      scopeType: 'session',
      scopeID: sessions[0].session_key,
    })
  },
  { immediate: true },
)

const editDialogOpen = ref(false)
const editingItem = ref<ProfileMemoryRecord | null>(null)
const editForm = ref({
  summary: '',
  value_json: '',
})

const refresh = async () => {
  await memoryStore.load()
}

async function selectSession(sessionKey: string) {
  await memoryStore.load({ scopeType: 'session', scopeID: sessionKey })
}

function formatLinks(links: MemoryLinkRecord[]) {
  if (!links || links.length === 0) return '[]'
  return JSON.stringify(links, null, 2)
}

function getConfirmationTone(state: ConfirmationState): 'default' | 'success' | 'warning' | 'error' | 'info' {
  switch (state) {
    case 'confirmed':
    case 'auto_confirmed':
      return 'success'
    case 'pending':
      return 'warning'
    case 'rejected':
      return 'error'
    default:
      return 'default'
  }
}

function getEffectiveTone(status: EffectiveStatus): 'default' | 'success' | 'warning' | 'error' | 'info' {
  switch (status) {
    case 'active':
      return 'success'
    case 'inactive':
      return 'default'
    case 'suppressed':
      return 'warning'
    case 'expired':
    case 'deleted':
      return 'error'
    default:
      return 'default'
  }
}

async function handleConfirm(type: MemoryType, id: number) {
  await memoryStore.confirm(type, id)
}

async function handleReject(type: MemoryType, id: number) {
  await memoryStore.reject(type, id)
}

async function handleSuppress(type: MemoryType, id: number) {
  await memoryStore.suppress(type, id)
}

async function handleUnsuppress(type: MemoryType, id: number) {
  await memoryStore.unsuppress(type, id)
}

async function handleDelete(type: MemoryType, id: number) {
  if (confirm('Are you sure you want to delete this memory entry?')) {
    await memoryStore.remove(type, id)
  }
}

function openEditDialog(item: ProfileMemoryRecord) {
  editingItem.value = item
  editForm.value = {
    summary: item.summary,
    value_json: item.value_json,
  }
  editDialogOpen.value = true
}

function closeEditDialog() {
  editDialogOpen.value = false
  editingItem.value = null
  editForm.value = { summary: '', value_json: '' }
}

function onDialogToggle(value: boolean) {
  if (!value) {
    closeEditDialog()
  }
}

async function submitEdit() {
  if (!editingItem.value) return
  await memoryStore.patch('profile', editingItem.value.id, {
    summary: editForm.value.summary,
    value_json: editForm.value.value_json,
  })
  closeEditDialog()
}
</script>

<style scoped>
.context-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-4);
  flex-wrap: wrap;
}

.recent-contexts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-3);
}

.context-card {
  display: grid;
  gap: var(--space-2);
  padding: var(--space-4);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  background: var(--color-bg-surfaceMuted);
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.context-card--active {
  border-color: var(--color-accent-primary);
  background: var(--color-accent-primaryMuted);
}

.context-card__title,
.context-card__meta {
  margin: 0;
}

.context-card__title {
  font-weight: var(--font-semibold);
}

.context-card__meta {
  color: var(--color-text-secondary);
  font-size: var(--text-sm);
}

.manual-scope {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-3);
  padding-top: var(--space-4);
  border-top: 1px solid var(--color-border-default);
}

.manual-scope label,
.memory-filters label,
.edit-form label {
  display: grid;
  gap: var(--space-2);
  color: var(--color-text-secondary);
  font-size: var(--text-sm);
}

.manual-scope__actions {
  display: flex;
  align-items: flex-end;
}

.memory-filters {
  display: flex;
  gap: var(--space-3);
  align-items: flex-end;
  flex-wrap: wrap;
}

.memory-toggle {
  display: inline-flex !important;
  align-items: center;
  gap: var(--space-2);
}

.memory-layout {
  display: grid;
  gap: var(--space-4);
}

.memory-card {
  display: grid;
  gap: var(--space-4);
  padding: var(--space-5);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
  background: var(--color-bg-surface);
}

.memory-card--primary {
  background: var(--color-bg-elevated);
}

.memory-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}

.memory-card__header h3,
.dialog-title {
  margin: 0;
}

.memory-card__copy,
.summary {
  margin: 0;
  color: var(--color-text-secondary);
}

.memory-list {
  display: grid;
  gap: var(--space-3);
}

.memory-item {
  display: grid;
  gap: var(--space-3);
  padding: var(--space-4);
  border-radius: var(--radius-md);
  border: 1px solid var(--color-border-default);
  background: var(--color-bg-surfaceMuted);
}

.memory-item--suppressed {
  opacity: 0.7;
  border-left: 3px solid var(--color-state-warning);
}

.memory-item header {
  display: flex;
  justify-content: space-between;
  gap: var(--space-3);
  align-items: flex-start;
  flex-wrap: wrap;
}

.badges {
  display: flex;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.memory-actions {
  display: flex;
  gap: var(--space-2);
  flex-wrap: wrap;
  margin-top: var(--space-2);
  padding-top: var(--space-2);
  border-top: 1px solid var(--color-border-default);
}

.edit-form {
  display: grid;
  gap: var(--space-4);
}

.json-textarea {
  width: 100%;
  min-width: 400px;
  background: var(--color-bg-surfaceMuted);
  color: var(--color-text-primary);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  padding: var(--space-2) var(--space-3);
  font-family: var(--font-mono);
  font-size: var(--text-sm);
  resize: vertical;
}

.dialog-actions {
  display: flex;
  gap: var(--space-2);
  justify-content: flex-end;
}

@media (max-width: 860px) {
  .memory-filters {
    display: grid;
  }

  .json-textarea {
    min-width: 100%;
  }
}
</style>
