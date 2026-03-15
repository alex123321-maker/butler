<template>
  <section class="settings-group">
    <button class="group-header" type="button" @click="open = !open">
      <div>
        <p class="group-label">{{ title }}</p>
        <h3>{{ group.name }}</h3>
      </div>
      <span class="group-toggle">{{ open ? '−' : '+' }}</span>
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
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 18px;
  background: linear-gradient(180deg, rgba(20, 28, 44, 0.92), rgba(14, 18, 30, 0.94));
  overflow: hidden;
}

.group-header {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px 22px;
  border: 0;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.group-label {
  margin: 0 0 6px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.18em;
  color: rgba(255, 255, 255, 0.48);
}

.group-header h3 {
  margin: 0;
  font-size: 22px;
}

.group-toggle {
  font-size: 28px;
  color: rgba(255, 255, 255, 0.65);
}

.group-body {
  display: grid;
  gap: 12px;
  padding: 0 18px 18px;
}
</style>
