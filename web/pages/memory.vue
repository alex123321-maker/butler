<template>
  <div class="memory-page">
    <section class="memory-hero">
      <div>
        <p class="eyebrow">Memory Browser</p>
        <h2 class="page-title">Memory</h2>
        <p class="hero-copy">Browse and manage durable profile, episodic, and chunk memory by scope.</p>
      </div>
      <div class="memory-controls">
        <label>
          Scope type
          <AppSelect v-model="filters.scopeType">
            <option value="session">session</option>
            <option value="user">user</option>
            <option value="global">global</option>
          </AppSelect>
        </label>
        <label>
          Scope id
          <AppInput v-model="filters.scopeID" type="text" placeholder="telegram:chat:123 or user id" />
        </label>
        <AppButton variant="primary" :disabled="pending" @click="refresh">Refresh</AppButton>
      </div>
    </section>

    <!-- Filters -->
    <section v-if="hasScope" class="memory-filters">
      <label>
        Confirmation State
        <AppSelect v-model="filters.confirmationState">
          <option value="all">All</option>
          <option value="pending">Pending</option>
          <option value="confirmed">Confirmed</option>
          <option value="rejected">Rejected</option>
          <option value="auto_confirmed">Auto-confirmed</option>
        </AppSelect>
      </label>
      <label>
        Effective Status
        <AppSelect v-model="filters.effectiveStatus">
          <option value="all">All</option>
          <option value="active">Active</option>
          <option value="inactive">Inactive</option>
          <option value="suppressed">Suppressed</option>
          <option value="expired">Expired</option>
          <option value="deleted">Deleted</option>
        </AppSelect>
      </label>
      <label class="checkbox-label">
        <input v-model="filters.showSuppressed" type="checkbox" />
        Show suppressed
      </label>
    </section>

    <AppAlert v-if="error" tone="error">{{ error }}</AppAlert>

    <p v-if="!hasScope" class="placeholder-text">Enter a scope id to browse memory.</p>
    <p v-else-if="pending" class="placeholder-text">Loading memory...</p>

    <div v-else class="memory-grid">
      <!-- Working Memory (read-only, no management) -->
      <section class="memory-card">
        <h3>Working Memory</h3>
        <div v-if="memoryData?.working" class="memory-fields">
          <MemoryField label="Goal" :value="memoryData.working.goal" />
          <MemoryField label="Status" :value="memoryData.working.status" />
          <MemoryField label="Source" :value="`${memoryData.working.source_type}:${memoryData.working.source_id}`" />
          <MemoryField label="Provenance" :value="memoryData.working.provenance" block />
          <MemoryField label="Entities" :value="memoryData.working.entities_json" block />
          <MemoryField label="Pending steps" :value="memoryData.working.pending_steps_json" block />
        </div>
        <p v-else class="placeholder-text">No working memory for this scope.</p>
      </section>

      <!-- Profile Memory -->
      <section class="memory-card">
        <h3>Profile Memory <span class="count">({{ filteredProfile.length }})</span></h3>
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

            <!-- Actions -->
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
        <p v-else class="placeholder-text">No profile memory entries.</p>
      </section>

      <!-- Episodic Memory -->
      <section class="memory-card">
        <h3>Episodic Memory <span class="count">({{ filteredEpisodic.length }})</span></h3>
        <div v-if="filteredEpisodic.length" class="memory-list">
          <article v-for="item in filteredEpisodic" :key="item.id" class="memory-item" :class="{ 'memory-item--suppressed': item.suppressed }">
            <header>
              <strong>{{ item.summary }}</strong>
              <div class="badges">
                <AppBadge :tone="getConfirmationTone(item.confirmation_state)">{{ item.confirmation_state }}</AppBadge>
                <AppBadge :tone="getEffectiveTone(item.effective_status)">{{ item.effective_status }}</AppBadge>
              </div>
            </header>
            <p>{{ item.content }}</p>
            <MemoryField label="Tags" :value="item.tags_json" block />
            <MemoryField label="Provenance" :value="item.provenance" block />
            <MemoryField v-if="item.edited_by" label="Edited by" :value="`${item.edited_by} at ${item.edited_at}`" />
            <MemoryField label="Links" :value="formatLinks(item.links)" block />

            <!-- Actions (episodic is not editable, but confirmable/suppressible/deletable) -->
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
        <p v-else class="placeholder-text">No episodic memory entries.</p>
      </section>

      <!-- Chunk Memory -->
      <section class="memory-card">
        <h3>Chunk Memory <span class="count">({{ filteredChunks.length }})</span></h3>
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

            <!-- Actions (chunks: suppressible and hard-deletable only) -->
            <div class="memory-actions">
              <template v-if="item.capabilities.suppressible">
                <AppButton v-if="!item.suppressed" variant="secondary" :disabled="actionPending" @click="handleSuppress('chunk', item.id)">Suppress</AppButton>
                <AppButton v-else variant="secondary" :disabled="actionPending" @click="handleUnsuppress('chunk', item.id)">Unsuppress</AppButton>
              </template>
              <AppButton v-if="item.capabilities.deletable" variant="danger" :disabled="actionPending" @click="handleDelete('chunk', item.id)">Delete</AppButton>
            </div>
          </article>
        </div>
        <p v-else class="placeholder-text">No chunk memory entries.</p>
      </section>
    </div>

    <!-- Edit Dialog for Profile Memory -->
    <AppDialog :model-value="editDialogOpen" @update:model-value="onDialogToggle">
      <template #title>
        <h3 class="dialog-title">Edit Profile Memory</h3>
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
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useMemoryStore } from '~/shared/model/stores/memory'
import type { MemoryLinkRecord, MemoryType, ConfirmationState, EffectiveStatus, ProfileMemoryRecord } from '~/entities/memory/api'
import AppAlert from '~/shared/ui/AppAlert.vue'
import AppBadge from '~/shared/ui/AppBadge.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppDialog from '~/shared/ui/AppDialog.vue'
import AppInput from '~/shared/ui/AppInput.vue'
import AppSelect from '~/shared/ui/AppSelect.vue'

useHead({ title: 'Memory - Butler' })

const memoryStore = useMemoryStore()
const { records: memoryData, pending, actionPending, error, filters, filteredProfile, filteredEpisodic, filteredChunks } = storeToRefs(memoryStore)

const hasScope = computed(() => filters.value.scopeID.trim().length > 0)

// Edit dialog state
const editDialogOpen = ref(false)
const editingItem = ref<ProfileMemoryRecord | null>(null)
const editForm = ref({
  summary: '',
  value_json: '',
})

const refresh = async () => {
  await memoryStore.load()
}

onMounted(async () => {
  await refresh()
})

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

// Action handlers
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

// Edit dialog handlers
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
.memory-page { display: grid; gap: 24px; }
.memory-hero {
  display: flex; justify-content: space-between; gap: 20px; padding: 24px; border-radius: 24px;
  background: linear-gradient(145deg, rgba(14, 18, 30, 0.94), rgba(8, 12, 20, 0.98));
  border: 1px solid rgba(255,255,255,0.08);
}
.eyebrow { margin: 0 0 8px; text-transform: uppercase; letter-spacing: 0.24em; font-size: 12px; color: rgba(255,255,255,0.5); }
.hero-copy { max-width: 680px; color: rgba(255,255,255,0.72); }
.memory-controls { display: grid; gap: 12px; min-width: 280px; }
.memory-controls label { display: grid; gap: 6px; font-size: 13px; color: rgba(255,255,255,0.72); }

.memory-filters {
  display: flex;
  gap: var(--space-4);
  align-items: flex-end;
  flex-wrap: wrap;
  padding: var(--space-4);
  background: var(--color-bg-surface);
  border-radius: var(--radius-md);
  border: 1px solid var(--color-border-default);
}
.memory-filters label {
  display: grid;
  gap: var(--space-1);
  font-size: 13px;
  color: var(--color-text-secondary);
}
.checkbox-label {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  cursor: pointer;
}

.memory-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(400px, 1fr)); gap: 18px; }
.memory-card { display: grid; gap: 14px; padding: 20px; border-radius: 20px; background: rgba(10, 16, 28, 0.88); border: 1px solid rgba(255,255,255,0.08); }
.memory-card h3 { margin: 0; display: flex; align-items: center; gap: var(--space-2); }
.memory-card .count { font-size: 14px; color: var(--color-text-secondary); font-weight: normal; }
.memory-list { display: grid; gap: 12px; }
.memory-item { display: grid; gap: 8px; padding: 14px; border-radius: 14px; background: rgba(255,255,255,0.04); }
.memory-item--suppressed { opacity: 0.6; border-left: 3px solid var(--color-state-warning); }
.memory-item header { display: flex; justify-content: space-between; gap: 12px; align-items: flex-start; flex-wrap: wrap; }
.memory-item .badges { display: flex; gap: var(--space-2); flex-wrap: wrap; }
.memory-item .summary { color: var(--color-text-secondary); margin: 0; }
.placeholder-text { color: rgba(255,255,255,0.64); }

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
.edit-form label {
  display: grid;
  gap: var(--space-1);
  font-size: 13px;
  color: var(--color-text-secondary);
}
.json-textarea {
  width: 100%;
  min-width: 400px;
  background: var(--color-bg-surfaceMuted);
  color: var(--color-text-primary);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-sm);
  padding: var(--space-2) var(--space-3);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  resize: vertical;
}

.dialog-title {
  margin: 0;
  font-size: 16px;
}

.dialog-actions {
  display: flex;
  gap: var(--space-2);
  justify-content: flex-end;
}

@media (max-width: 860px) { .memory-hero { flex-direction: column; } }
</style>
