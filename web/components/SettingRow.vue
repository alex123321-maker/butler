<template>
  <article class="setting-row">
    <div class="setting-main">
      <div class="setting-head">
        <code>{{ setting.key }}</code>
        <div class="badges">
          <span class="badge" :class="`source-${setting.source}`">{{ setting.source }}</span>
          <span class="badge" :class="`validation-${setting.validation_status}`">{{ setting.validation_status }}</span>
          <span v-if="restartPending" class="badge restart">restart required</span>
        </div>
      </div>

      <div v-if="editing" class="editor">
        <select v-if="hasAllowedValues" v-model="draft" class="setting-input">
          <option v-for="value in setting.allowed_values" :key="value" :value="value">{{ value }}</option>
        </select>
        <input
          v-else
          v-model="draft"
          class="setting-input"
          :type="setting.is_secret && !showSecret ? 'password' : 'text'"
          :placeholder="setting.is_secret ? 'Enter new secret value' : 'Enter new value'"
        >
        <button v-if="setting.is_secret" class="ghost-btn" type="button" @click="showSecret = !showSecret">
          {{ showSecret ? 'Hide' : 'Show' }}
        </button>
      </div>
      <p v-else class="setting-value">{{ setting.value || '—' }}</p>

      <p v-if="setting.validation_error" class="setting-error">{{ setting.validation_error }}</p>
    </div>

    <div class="setting-actions">
      <template v-if="editing">
        <button class="primary-btn" type="button" :disabled="saving" @click="$emit('save', { key: setting.key, value: draft })">Save</button>
        <button class="ghost-btn" type="button" :disabled="saving" @click="cancel">Cancel</button>
      </template>
      <template v-else>
        <button class="ghost-btn" type="button" :disabled="saving" @click="startEdit">Edit</button>
        <button v-if="setting.source === 'db'" class="ghost-btn danger" type="button" :disabled="saving" @click="$emit('remove', setting.key)">Delete</button>
      </template>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { SettingItem } from '~/composables/useSettings'

const emit = defineEmits<{
  save: [{ key: string, value: string }]
  remove: [key: string]
}>()

const props = defineProps<{
  setting: SettingItem
  saving: boolean
  restartPending: boolean
}>()

const editing = ref(false)
const draft = ref('')
const showSecret = ref(false)
const hasAllowedValues = computed(() => (props.setting.allowed_values?.length ?? 0) > 0)

watch(() => props.setting.value, (value) => {
  if (!editing.value) {
    draft.value = value
  }
}, { immediate: true })

function startEdit() {
  editing.value = true
  draft.value = props.setting.value
  showSecret.value = false
}

function cancel() {
  editing.value = false
  draft.value = props.setting.value
  showSecret.value = false
}

watch(() => props.saving, (saving) => {
  if (!saving) {
    editing.value = false
  }
})
</script>

<style scoped>
.setting-row {
  display: flex;
  gap: var(--space-4);
  justify-content: space-between;
  padding: var(--space-4);
  border-radius: var(--radius-lg);
  background: var(--color-bg-surface);
  border: 1px solid var(--color-border-default);
}

.setting-main {
  flex: 1;
  min-width: 0;
}

.setting-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3);
}

.setting-head code {
  font-size: var(--text-xs);
  color: var(--color-text-primary);
}

.badges {
  display: flex;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.badge {
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-full);
  font-size: var(--text-xs);
  text-transform: uppercase;
  letter-spacing: var(--tracking-wider);
}

.source-env { background: var(--color-accent-primaryMuted); color: var(--color-accent-primaryHover); }
.source-db { background: var(--color-state-successMuted); color: var(--color-state-success); }
.source-default { background: var(--color-state-neutralMuted); color: var(--color-text-secondary); }
.validation-valid { background: var(--color-state-successMuted); color: var(--color-state-success); }
.validation-invalid, .validation-missing { background: var(--color-state-errorMuted); color: var(--color-state-error); }
.restart { background: var(--color-state-warningMuted); color: var(--color-state-warning); }

.setting-value {
  margin: var(--space-4) 0 0;
  color: var(--color-text-secondary);
  word-break: break-word;
}

.editor {
  display: flex;
  gap: var(--space-2);
  margin-top: var(--space-4);
}

.setting-input {
  flex: 1;
  min-width: 0;
  border-radius: var(--radius-md);
  border: 1px solid var(--color-border-default);
  background: var(--color-bg-canvas);
  color: var(--color-text-primary);
  padding: var(--space-3) var(--space-4);
}

.setting-actions {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.primary-btn,
.ghost-btn {
  min-width: 88px;
  border-radius: var(--radius-full);
  padding: var(--space-3) var(--space-4);
  cursor: pointer;
}

.primary-btn {
  border: 1px solid var(--color-accent-primary);
  background: var(--color-accent-primary);
  color: var(--color-text-inverse);
}

.ghost-btn {
  border: 1px solid var(--color-border-default);
  background: var(--color-bg-surfaceMuted);
  color: var(--color-text-primary);
}

.danger {
  color: var(--color-state-error);
}

.setting-error {
  margin: var(--space-2) 0 0;
  color: var(--color-state-error);
  font-size: var(--text-sm);
}

@media (max-width: 860px) {
  .setting-row {
    flex-direction: column;
  }

  .setting-actions {
    flex-direction: row;
    flex-wrap: wrap;
  }

  .editor {
    flex-direction: column;
  }
}
</style>
