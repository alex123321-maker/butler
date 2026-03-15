<template>
  <div class="settings-page">
    <section class="settings-hero">
      <div>
        <p class="eyebrow">Control Room</p>
        <h2 class="page-title">Settings</h2>
        <p class="hero-copy">Review effective configuration, trace where each value comes from, and apply overrides without leaving the dashboard.</p>
      </div>
      <button class="refresh-btn" type="button" :disabled="pending || busyKey !== null" @click="refresh">Refresh</button>
    </section>

    <p v-if="toast" class="toast">{{ toast }}</p>
    <p v-if="pending" class="placeholder-text">Loading settings...</p>
    <p v-else-if="error" class="placeholder-text">Failed to load settings.</p>
    <p v-else-if="!groups.length" class="placeholder-text">No settings available.</p>

    <div v-else class="settings-grid">
      <SettingsGroup
        v-for="group in groups"
        :key="group.name"
        :group="group"
        :saving-key="busyKey"
        :restart-keys="restartKeys"
        @save="saveSetting"
        @remove="removeSetting"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import SettingsGroup from '~/components/SettingsGroup.vue'
import { useSettingsData, type SettingsComponent } from '~/composables/useSettings'

useHead({ title: 'Settings — Butler' })

const { data, pending, error, refresh, updateSetting, deleteSetting } = useSettingsData()
const busyKey = ref<string | null>(null)
const toast = ref('')
const restartKeys = ref(new Set<string>())

const groups = computed<SettingsComponent[]>(() => data.value ?? [])

function replaceSetting(nextSetting: any) {
  const nextGroups = (data.value ?? []).map((group) => ({
    ...group,
    settings: group.settings.map((setting) => setting.key === nextSetting.key ? nextSetting : setting),
  }))
  if (!nextGroups.some((group) => group.settings.some((setting) => setting.key === nextSetting.key))) {
    nextGroups.push({ name: nextSetting.component, settings: [nextSetting] })
  }
  data.value = nextGroups.sort((a, b) => a.name.localeCompare(b.name))
}

async function saveSetting(payload: { key: string, value: string }) {
  busyKey.value = payload.key
  toast.value = ''
  try {
    const setting = await updateSetting(payload.key, payload.value)
    replaceSetting(setting)
    if (setting.requires_restart) {
      restartKeys.value = new Set([...restartKeys.value, setting.key])
    }
  } catch (err: any) {
    toast.value = err?.data?.error || err?.message || 'Failed to save setting.'
  } finally {
    busyKey.value = null
  }
}

async function removeSetting(key: string) {
  busyKey.value = key
  toast.value = ''
  try {
    const setting = await deleteSetting(key)
    replaceSetting(setting)
    const next = new Set(restartKeys.value)
    next.delete(key)
    restartKeys.value = next
  } catch (err: any) {
    toast.value = err?.data?.error || err?.message || 'Failed to delete setting.'
  } finally {
    busyKey.value = null
  }
}
</script>

<style scoped>
.settings-page {
  display: grid;
  gap: 24px;
}

.settings-hero {
  display: flex;
  justify-content: space-between;
  gap: 20px;
  padding: 28px;
  border-radius: 24px;
  background:
    radial-gradient(circle at top right, rgba(249, 115, 22, 0.28), transparent 32%),
    linear-gradient(145deg, rgba(14, 18, 30, 0.94), rgba(8, 12, 20, 0.98));
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.eyebrow {
  margin: 0 0 8px;
  text-transform: uppercase;
  letter-spacing: 0.24em;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
}

.hero-copy {
  max-width: 640px;
  color: rgba(255, 255, 255, 0.72);
}

.refresh-btn {
  align-self: flex-start;
  border: 0;
  border-radius: 999px;
  padding: 12px 18px;
  background: #f97316;
  color: #fff;
  cursor: pointer;
}

.settings-grid {
  display: grid;
  gap: 18px;
}

.toast {
  margin: 0;
  padding: 14px 16px;
  border-radius: 14px;
  background: rgba(231, 76, 60, 0.16);
  color: #ff9a8f;
}

@media (max-width: 860px) {
  .settings-hero {
    flex-direction: column;
  }
}
</style>
