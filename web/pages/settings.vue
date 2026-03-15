<template>
  <div class="settings-page">
    <section class="settings-hero">
      <div>
        <p class="eyebrow">Control Room</p>
        <h2 class="page-title">Settings</h2>
        <p class="hero-copy">Review effective configuration, trace where each value comes from, and apply overrides without leaving the dashboard.</p>
      </div>
      <div class="hero-actions">
        <button class="refresh-btn" type="button" :disabled="pending || busyKey !== null" @click="refreshAll">Refresh</button>
        <button class="refresh-btn restart-btn" type="button" :disabled="busyKey !== null || restartBusy || !restartState.components.length" @click="restartChanged">
          Restart changed
        </button>
      </div>
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

      <section class="tools-registry">
        <div class="tools-header">
          <div>
            <p class="group-label">Tools</p>
            <h3>tools.json</h3>
            <p class="registry-path">{{ toolsPath }}</p>
          </div>
          <button class="refresh-btn" type="button" :disabled="toolsBusy" @click="loadToolsRegistry">Reload file</button>
        </div>
        <textarea v-model="toolsContent" class="tools-editor" spellcheck="false" />
        <div class="tools-actions">
          <button class="primary-btn" type="button" :disabled="toolsBusy" @click="saveToolsRegistry">Save tools.json</button>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import SettingsGroup from '~/components/SettingsGroup.vue'
import { useSettingsData, type SettingsComponent } from '~/composables/useSettings'

useHead({ title: 'Settings — Butler' })

const { data, pending, error, refresh, updateSetting, deleteSetting, getToolsRegistry, updateToolsRegistry, getRestartState, applyRestart } = useSettingsData()
const busyKey = ref<string | null>(null)
const toast = ref('')
const restartKeys = ref(new Set<string>())
const restartBusy = ref(false)
const restartState = ref({ components: [] as string[], suggested_command: '' })
const toolsBusy = ref(false)
const toolsContent = ref('')
const toolsPath = ref('')

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
    await refreshRestartState()
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
    await refreshRestartState()
  } catch (err: any) {
    toast.value = err?.data?.error || err?.message || 'Failed to delete setting.'
  } finally {
    busyKey.value = null
  }
}

async function loadToolsRegistry() {
  toolsBusy.value = true
  toast.value = ''
  try {
    const payload = await getToolsRegistry()
    toolsPath.value = payload.path
    toolsContent.value = payload.content
  } catch (err: any) {
    toast.value = err?.data?.error || err?.message || 'Failed to load tools.json.'
  } finally {
    toolsBusy.value = false
  }
}

async function saveToolsRegistry() {
  toolsBusy.value = true
  toast.value = ''
  try {
    const payload = await updateToolsRegistry(toolsContent.value)
    toolsPath.value = payload.path
    await refreshRestartState()
  } catch (err: any) {
    toast.value = err?.data?.error || err?.message || 'Failed to save tools.json.'
  } finally {
    toolsBusy.value = false
  }
}

async function refreshRestartState() {
  const payload = await getRestartState()
  restartState.value = payload
  const activeKeys = new Set<string>()
  const componentSet = new Set(payload.components)
  for (const group of groups.value) {
    for (const setting of group.settings) {
      if (setting.requires_restart && componentSet.has(setting.component)) {
        activeKeys.add(setting.key)
      }
    }
  }
  restartKeys.value = activeKeys
}

async function refreshAll() {
  await refresh()
  await Promise.all([refreshRestartState(), loadToolsRegistry()])
}

async function restartChanged() {
  restartBusy.value = true
  toast.value = ''
  try {
    const payload = await applyRestart()
    restartState.value = payload
    restartKeys.value = new Set()
    if (payload.suggested_command) {
      toast.value = `Run command to restart changed services: ${payload.suggested_command}`
    }
  } catch (err: any) {
    toast.value = err?.data?.error || err?.message || 'Failed to build restart command.'
  } finally {
    restartBusy.value = false
  }
}

await Promise.all([refreshRestartState(), loadToolsRegistry()])
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

.hero-actions {
  display: flex;
  gap: 10px;
}

.restart-btn {
  background: linear-gradient(135deg, #c2410c, #f97316);
}

.settings-grid {
  display: grid;
  gap: 18px;
}

.tools-registry {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 18px;
  background: linear-gradient(180deg, rgba(20, 28, 44, 0.92), rgba(14, 18, 30, 0.94));
  padding: 20px;
  display: grid;
  gap: 12px;
}

.tools-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
}

.tools-header h3 {
  margin: 0;
  font-size: 22px;
}

.registry-path {
  margin: 6px 0 0;
  color: rgba(255, 255, 255, 0.62);
  font-size: 12px;
}

.tools-editor {
  width: 100%;
  min-height: 260px;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.14);
  background: rgba(7, 10, 18, 0.72);
  color: #f8fafc;
  padding: 12px 14px;
  font-family: "JetBrains Mono", "Fira Code", monospace;
  font-size: 13px;
}

.tools-actions {
  display: flex;
  justify-content: flex-end;
}

.primary-btn {
  border: 0;
  border-radius: 999px;
  padding: 10px 14px;
  min-width: 88px;
  background: linear-gradient(135deg, #f97316, #fb7185);
  color: #fff;
  cursor: pointer;
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

  .hero-actions,
  .tools-header {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
