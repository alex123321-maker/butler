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

    <div v-if="providerState.providers.length" class="provider-panel">
      <div class="provider-panel__header">
        <div>
          <p class="provider-panel__eyebrow">Provider Access</p>
          <h3>Connected model providers</h3>
          <p>Pick the active provider in settings, then connect OAuth-backed providers here without exposing tokens in the UI.</p>
        </div>
      </div>

      <div class="provider-list">
        <article v-for="provider in providerState.providers" :key="provider.name" class="provider-card" :class="{ 'provider-card--active': provider.active }">
          <div class="provider-card__topline">
            <div>
              <h4>{{ providerTitle(provider.name) }}</h4>
              <p>{{ provider.model }}</p>
            </div>
            <span class="provider-badge" :class="provider.connected ? 'provider-badge--connected' : 'provider-badge--idle'">
              {{ provider.connected ? 'Connected' : provider.auth_kind === 'api_key' ? 'Config only' : 'Not connected' }}
            </span>
          </div>

          <p class="provider-meta">
            <span v-if="provider.active">Active provider</span>
            <span v-if="provider.account_hint">{{ provider.account_hint }}</span>
            <span v-if="provider.enterprise_domain">{{ provider.enterprise_domain }}</span>
            <span v-if="provider.expires_at">Expires {{ formatDate(provider.expires_at) }}</span>
          </p>

          <div v-if="provider.pending" class="provider-flow">
            <p class="provider-flow__status">{{ provider.pending.status.replaceAll('_', ' ') }}</p>
            <p v-if="provider.pending.instructions" class="provider-flow__text">{{ provider.pending.instructions }}</p>
            <div v-if="provider.pending.verification_uri" class="provider-flow__callout">
              <span>Verification URL</span>
              <a :href="provider.pending.verification_uri" target="_blank" rel="noopener noreferrer">{{ provider.pending.verification_uri }}</a>
            </div>
            <div v-if="provider.pending.auth_url" class="provider-flow__callout">
              <span>Authorization URL</span>
              <a :href="provider.pending.auth_url" target="_blank" rel="noopener noreferrer">Open sign-in flow</a>
            </div>
            <div v-if="provider.pending.user_code" class="provider-flow__code">{{ provider.pending.user_code }}</div>
            <p v-if="provider.pending.error" class="provider-flow__error">{{ provider.pending.error }}</p>

            <div v-if="provider.name === 'openai-codex'" class="provider-flow__complete">
              <textarea
                v-model="codexInput"
                class="provider-textarea"
                rows="3"
                placeholder="Paste the full redirect URL or authorization code"
              />
              <button
                class="provider-btn"
                type="button"
                :disabled="providerBusy === provider.name || !provider.pending.id || !codexInput.trim()"
                @click="completeCodex(provider.name, provider.pending.id)"
              >
                Complete OpenAI Codex login
              </button>
            </div>
          </div>

          <div class="provider-actions">
            <template v-if="provider.name === 'openai'">
              <p class="provider-actions__hint">Managed through `BUTLER_OPENAI_API_KEY` and grouped settings.</p>
            </template>
            <template v-else-if="provider.name === 'github-copilot'">
              <input
                v-model="enterpriseInput"
                class="provider-input"
                type="text"
                placeholder="GitHub Enterprise URL (optional)"
              />
              <button class="provider-btn" type="button" :disabled="providerBusy === provider.name" @click="startProvider(provider.name)">
                {{ provider.connected ? 'Reconnect' : 'Start device flow' }}
              </button>
              <button v-if="provider.connected || provider.pending" class="provider-btn provider-btn--ghost" type="button" :disabled="providerBusy === provider.name" @click="disconnectProvider(provider.name)">
                Disconnect
              </button>
            </template>
            <template v-else>
              <button class="provider-btn" type="button" :disabled="providerBusy === provider.name" @click="startProvider(provider.name)">
                {{ provider.connected ? 'Reconnect' : 'Start OAuth flow' }}
              </button>
              <button v-if="provider.connected || provider.pending" class="provider-btn provider-btn--ghost" type="button" :disabled="providerBusy === provider.name" @click="disconnectProvider(provider.name)">
                Disconnect
              </button>
            </template>
          </div>
        </article>
      </div>
    </div>

    <div v-if="groups.length" class="settings-grid">
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
import { useSettingsData, type ProviderItem, type SettingsComponent } from '~/composables/useSettings'

useHead({ title: 'Settings — Butler' })

const { data, pending, error, refresh, updateSetting, deleteSetting, getRestartState, applyRestart, getProviders, startProviderAuth, completeProviderAuth, deleteProviderAuth } = useSettingsData()
const busyKey = ref<string | null>(null)
const providerBusy = ref<string | null>(null)
const toast = ref('')
const restartKeys = ref(new Set<string>())
const restartBusy = ref(false)
const restartState = ref({ components: [] as string[], suggested_command: '' })
const providerState = ref({ active_provider: '', providers: [] as ProviderItem[] })
const enterpriseInput = ref('')
const codexInput = ref('')

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
  await refreshProviders()
  await refreshRestartState()
}

function providerTitle(name: string) {
  switch (name) {
    case 'github-copilot':
      return 'GitHub Copilot'
    case 'openai-codex':
      return 'OpenAI Codex'
    case 'openai':
      return 'OpenAI API'
    default:
      return name
  }
}

function formatDate(value: string) {
  return new Date(value).toLocaleString()
}

async function refreshProviders() {
  providerState.value = await getProviders()
}

async function startProvider(name: string) {
  providerBusy.value = name
  toast.value = ''
  try {
    const response = await startProviderAuth(name, name === 'github-copilot' ? enterpriseInput.value : '')
    await refreshProviders()
    if (response.flow?.verification_uri) {
      toast.value = `Open ${response.flow.verification_uri} and enter code ${response.flow.user_code ?? ''}`.trim()
    } else if (response.flow?.auth_url) {
      toast.value = 'Open the authorization URL to continue the OAuth flow.'
    }
  } catch (err: any) {
    toast.value = err?.data?.error || err?.message || 'Failed to start provider login.'
  } finally {
    providerBusy.value = null
  }
}

async function completeCodex(name: string, flowId: string) {
  providerBusy.value = name
  toast.value = ''
  try {
    await completeProviderAuth(name, flowId, codexInput.value)
    codexInput.value = ''
    await refreshProviders()
    toast.value = 'OpenAI Codex provider connected.'
  } catch (err: any) {
    toast.value = err?.data?.error || err?.message || 'Failed to complete provider login.'
  } finally {
    providerBusy.value = null
  }
}

async function disconnectProvider(name: string) {
  providerBusy.value = name
  toast.value = ''
  try {
    await deleteProviderAuth(name)
    await refreshProviders()
    if (name === 'openai-codex') {
      codexInput.value = ''
    }
    toast.value = `${providerTitle(name)} disconnected.`
  } catch (err: any) {
    toast.value = err?.data?.error || err?.message || 'Failed to disconnect provider.'
  } finally {
    providerBusy.value = null
  }
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

await refreshProviders()
await refreshRestartState()
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

.provider-panel {
  display: grid;
  gap: 16px;
  padding: 24px;
  border-radius: 24px;
  background: rgba(10, 16, 28, 0.88);
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.provider-panel__header h3 {
  margin: 0;
  font-size: 22px;
}

.provider-panel__header p {
  margin: 8px 0 0;
  color: rgba(255, 255, 255, 0.7);
}

.provider-panel__eyebrow {
  margin: 0 0 8px;
  text-transform: uppercase;
  letter-spacing: 0.22em;
  font-size: 11px;
  color: rgba(255, 255, 255, 0.48);
}

.provider-list {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 16px;
}

.provider-card {
  display: grid;
  gap: 14px;
  padding: 18px;
  border-radius: 18px;
  background: linear-gradient(180deg, rgba(17, 24, 39, 0.94), rgba(10, 15, 25, 0.92));
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.provider-card--active {
  border-color: rgba(249, 115, 22, 0.55);
  box-shadow: 0 0 0 1px rgba(249, 115, 22, 0.18);
}

.provider-card__topline {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.provider-card__topline h4 {
  margin: 0;
}

.provider-card__topline p {
  margin: 4px 0 0;
  color: rgba(255, 255, 255, 0.65);
}

.provider-badge {
  align-self: flex-start;
  padding: 6px 10px;
  border-radius: 999px;
  font-size: 12px;
}

.provider-badge--connected {
  background: rgba(34, 197, 94, 0.18);
  color: #8df0af;
}

.provider-badge--idle {
  background: rgba(148, 163, 184, 0.16);
  color: rgba(255, 255, 255, 0.72);
}

.provider-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 0;
  color: rgba(255, 255, 255, 0.62);
  font-size: 13px;
}

.provider-meta span {
  padding: 4px 8px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.provider-actions {
  display: grid;
  gap: 10px;
}

.provider-actions__hint {
  margin: 0;
  color: rgba(255, 255, 255, 0.62);
  font-size: 13px;
}

.provider-input,
.provider-textarea {
  width: 100%;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.03);
  color: #fff;
  padding: 12px 14px;
}

.provider-textarea {
  resize: vertical;
  min-height: 88px;
}

.provider-btn {
  border: 0;
  border-radius: 12px;
  padding: 12px 14px;
  background: #f97316;
  color: #fff;
  cursor: pointer;
}

.provider-btn--ghost {
  background: rgba(255, 255, 255, 0.08);
}

.provider-btn:disabled,
.refresh-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.provider-flow {
  display: grid;
  gap: 10px;
  padding: 14px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.04);
}

.provider-flow__status {
  margin: 0;
  text-transform: capitalize;
  color: #ffd0b2;
}

.provider-flow__text,
.provider-flow__error {
  margin: 0;
  color: rgba(255, 255, 255, 0.72);
}

.provider-flow__error {
  color: #ff9a8f;
}

.provider-flow__callout {
  display: grid;
  gap: 4px;
}

.provider-flow__callout span {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
  text-transform: uppercase;
  letter-spacing: 0.12em;
}

.provider-flow__callout a {
  color: #fbbf24;
  word-break: break-all;
}

.provider-flow__code {
  display: inline-flex;
  width: fit-content;
  padding: 8px 12px;
  border-radius: 10px;
  background: rgba(249, 115, 22, 0.16);
  color: #ffd0b2;
  font-weight: 600;
  letter-spacing: 0.08em;
}

.provider-flow__complete {
  display: grid;
  gap: 10px;
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

  .hero-actions {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
