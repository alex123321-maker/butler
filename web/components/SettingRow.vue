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
  gap: 18px;
  justify-content: space-between;
  padding: 18px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.05);
}

.setting-main {
  flex: 1;
  min-width: 0;
}

.setting-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}

.setting-head code {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.88);
}

.badges {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.badge {
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.12em;
}

.source-env { background: rgba(79, 140, 255, 0.16); color: #72a7ff; }
.source-db { background: rgba(46, 204, 113, 0.16); color: #54d48a; }
.source-default { background: rgba(148, 163, 184, 0.18); color: #cbd5e1; }
.validation-valid { background: rgba(46, 204, 113, 0.16); color: #54d48a; }
.validation-invalid, .validation-missing { background: rgba(231, 76, 60, 0.16); color: #ff8677; }
.restart { background: rgba(243, 156, 18, 0.18); color: #f8bf60; }

.setting-value {
  margin: 14px 0 0;
  color: rgba(255, 255, 255, 0.74);
  word-break: break-word;
}

.editor {
  display: flex;
  gap: 10px;
  margin-top: 14px;
}

.setting-input {
  flex: 1;
  min-width: 0;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.14);
  background: rgba(7, 10, 18, 0.72);
  color: #f8fafc;
  padding: 12px 14px;
}

.setting-actions {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.primary-btn,
.ghost-btn {
  min-width: 88px;
  border-radius: 999px;
  padding: 10px 14px;
  cursor: pointer;
}

.primary-btn {
  border: 0;
  background: linear-gradient(135deg, #f97316, #fb7185);
  color: #fff;
}

.ghost-btn {
  border: 1px solid rgba(255, 255, 255, 0.14);
  background: transparent;
  color: rgba(255, 255, 255, 0.82);
}

.danger {
  color: #ff8677;
}

.setting-error {
  margin: 10px 0 0;
  color: #ff8677;
  font-size: 13px;
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
