<template>
  <section class="settings-group">
    <button class="group-header" type="button" @click="open = !open">
      <div>
        <p class="group-label">{{ title }}</p>
        <h3>{{ group.name }}</h3>
      </div>
      <span class="group-toggle">{{ open ? '-' : '+' }}</span>
    </button>

    <div v-if="open" class="group-body">
      <SettingRow
        v-for="setting in group.settings"
        :key="setting.key"
        :setting="setting"
        :saving="savingKey === setting.key"
        :restart-pending="restartKeys.has(setting.key)"
        @save="$emit('save', $event)"
        @remove="$emit('remove', $event)"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { SettingsComponent } from '~/composables/useSettings'

defineEmits<{
  save: [{ key: string, value: string }]
  remove: [key: string]
}>()

const props = defineProps<{
  group: SettingsComponent
  savingKey: string | null
  restartKeys: Set<string>
}>()

const open = ref(true)

const title = computed(() => `${props.group.settings.length} settings`)
</script>

<style scoped>
.settings-group {
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
  background: var(--color-bg-surfaceMuted);
  overflow: hidden;
}

.group-header {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-5);
  border: 0;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.group-label {
  margin: 0 0 var(--space-2);
  font-size: var(--text-xs);
  text-transform: uppercase;
  letter-spacing: var(--tracking-wider);
  color: var(--color-text-muted);
}

.group-header h3 {
  margin: 0;
  font-size: var(--text-2xl);
}

.group-toggle {
  font-size: var(--text-3xl);
  color: var(--color-text-secondary);
}

.group-body {
  display: grid;
  gap: var(--space-3);
  padding: 0 var(--space-4) var(--space-4);
}
</style>
