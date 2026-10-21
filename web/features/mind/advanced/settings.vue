<template><div class="mind-advanced">
  <div class="settings-page">
    <section class="settings-hero">
      <div>
        <p class="eyebrow">Settings</p>

        <p class="hero-copy">Manage integrations, policies, and prompts in the same workspace style as the rest of Butler.</p>
      </div>
      <div class="hero-actions">
        <button class="refresh-btn" type="button" :disabled="pending || busyKey !== null" @click="refreshAll">Refresh</button>
        <button class="refresh-btn restart-btn" type="button" :disabled="busyKey !== null || restartBusy || !restartState.components.length" @click="restartChanged">
          Restart changed
        </button>
      </div>
    </section>

    <label class="owner-maintenance-token">Owner control token
      <input v-model="ownerToken" type="password" autocomplete="off" aria-label="Owner control token" />
      <span>Required to change settings, provider authentication or system instructions.</span>
    </label>

    <p v-if="toast" class="toast">{{ toast }}</p>
    <p v-if="pending" class="placeholder-text">Loading settings...</p>
    <p v-else-if="error" class="placeholder-text">Failed to load settings.</p>
    <p v-else-if="!allGroups.length" class="placeholder-text">No settings available.</p>

    <section v-if="allGroups.length" class="settings-ia">
      <div class="settings-ia__header">
        <div>
          <p class="provider-panel__eyebrow">Information Architecture</p>
          <h3>Find policy toggles quickly</h3>
          <p>Filter by source, restart impact, and policy-related settings without digging through every component group.</p>
        </div>
        <div class="settings-ia__meta">
          <span class="provider-badge provider-badge--idle">{{ filteredSettingsCount }} shown</span>
          <span class="provider-badge provider-badge--idle">{{ totalSettingsCount }} total</span>
        </div>
      </div>

      <div class="settings-ia__controls">
        <input v-model="settingsQuery" class="provider-input" type="text" placeholder="Search by key, component, or group">
        <select v-model="sourceFilter" class="provider-input">
          <option value="all">All sources</option>
          <option value="env">env</option>
          <option value="db">db</option>
          <option value="default">default</option>
        </select>
        <label class="settings-ia__toggle">
          <input v-model="onlyRestart" type="checkbox">
          <span>Restart required only</span>
        </label>
      </div>

      <div class="settings-ia__segments">
        <button
          v-for="segment in focusSegments"
          :key="segment.value"
          class="placeholder-chip"
          :class="{ 'placeholder-chip--active': focusSegment === segment.value }"
          type="button"
          @click="focusSegment = segment.value"
        >
          {{ segment.label }}
        </button>
      </div>
    </section>

    <section v-if="policyHighlights.length" class="policy-panel">
      <div class="policy-panel__header">
        <div>
          <p class="provider-panel__eyebrow">Policy Highlights</p>
          <h3>Key policy and risk-related toggles</h3>
        </div>
      </div>
      <div class="policy-grid">
        <article v-for="item in policyHighlights" :key="item.key" class="policy-card">
          <p class="policy-card__key">{{ item.key }}</p>
          <p class="policy-card__meta">{{ item.component }} • {{ item.source }}</p>
          <p class="policy-card__value">{{ item.value || '—' }}</p>
          <span v-if="item.requires_restart" class="provider-badge provider-badge--idle">restart required</span>
        </article>
      </div>
    </section>

    <details class="policy-panel"><summary>Legacy user-tab extension (optional)</summary>
      <div class="policy-panel__header">
        <div>
          <p class="provider-panel__eyebrow">Browser Extension Remote</p>
          <h3>Remote onboarding checklist</h3>
          <p>Use these values to connect the Chromium extension to a non-localhost Butler deployment.</p>
        </div>
      </div>
      <div class="policy-grid extension-grid">
        <article class="policy-card">
          <p class="policy-card__key">Transport mode</p>
          <p class="policy-card__value">{{ browserControl.transportMode }}</p>
          <span class="provider-badge" :class="browserControl.transportMode === 'remote_preferred' ? 'provider-badge--connected' : 'provider-badge--idle'">
            {{ browserControl.transportMode === 'remote_preferred' ? 'remote-first' : 'not remote-first' }}
          </span>
        </article>
        <article class="policy-card">
          <p class="policy-card__key">Extension API tokens</p>
          <p class="policy-card__value">{{ browserControl.extensionAuthConfigured ? 'configured' : 'not configured' }}</p>
          <span class="provider-badge" :class="browserControl.extensionAuthConfigured ? 'provider-badge--connected' : 'provider-badge--idle'">
            {{ browserControl.extensionAuthConfigured ? 'ready' : 'required' }}
          </span>
        </article>
        <article class="policy-card">
          <p class="policy-card__key">Relay heartbeat TTL</p>
          <p class="policy-card__value">{{ browserControl.heartbeatTTL }} seconds</p>
          <p class="policy-card__meta">Key: `BUTLER_SINGLE_TAB_RELAY_HEARTBEAT_TTL_SECONDS`</p>
        </article>
        <article class="policy-card">
          <p class="policy-card__key">Recommended extension values</p>
          <p class="policy-card__meta">Rollout mode</p>
          <p class="policy-card__value">`remote_preferred`</p>
          <p class="policy-card__meta">Remote Butler URL</p>
          <p class="policy-card__value">{{ extensionRemoteURL || 'Set public.apiBase in Web runtime config' }}</p>
        </article>
      </div>
      <div class="extension-download">
        <a class="provider-btn extension-download__button" href="/downloads/chromium-butler-extension.zip" download>
          Download extension (.zip)
        </a>
        <p class="extension-download__hint">
          Download, unpack, and load the folder in Chrome/Chromium/Edge as an unpacked extension.
        </p>
      </div>
      <ol class="extension-install-steps">
        <li>Open `chrome://extensions` (or `edge://extensions`) and enable Developer mode.</li>
        <li>Click “Load unpacked” and select the extracted extension folder.</li>
        <li>Open the extension popup and set `Rollout mode = remote_preferred`.</li>
        <li>Fill `Remote Butler URL` and `Remote API token`, then click `Connect relay`.</li>
        <li>Enable the optional user-tab tool profile before asking the agent to bind a tab. The standard server browser is available from Browser in the sidebar.</li>
      </ol>
      <ul class="warning-list extension-steps">
        <li>Set `BUTLER_EXTENSION_API_TOKENS` and copy one token into extension popup field “Remote API token”.</li>
        <li>Set `BUTLER_SINGLE_TAB_TRANSPORT_MODE=remote_preferred` for remote-first rollout.</li>
        <li>Set extension popup `Rollout mode = remote_preferred` and `Remote Butler URL` to this server.</li>
      </ul>
    </details>

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
            <template v-else-if="provider.name === 'antigravity'">
              <p class="provider-actions__hint">Google AI Pro credentials are installed by the operator through a private sign-in and encrypted import. Connected means credentials are stored; model access also depends on your subscription quota.</p>
              <button v-if="provider.connected" class="provider-btn" type="button" :disabled="providerBusy === provider.name" @click="disconnectProvider(provider.name)">Disconnect</button>
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

    <div class="prompt-panel">
      <div class="prompt-panel__header">
        <div>
          <p class="provider-panel__eyebrow">Prompt Management</p>
          <h3>System prompt</h3>
          <p>Edit the operator base prompt, inject safe runtime sections, and preview the effective instruction before saving.</p>
        </div>
        <div class="prompt-panel__meta">
          <span class="provider-badge" :class="promptState.enabled ? 'provider-badge--connected' : 'provider-badge--idle'">
            {{ promptState.enabled ? 'Enabled' : 'Fallback default' }}
          </span>
          <span class="provider-badge provider-badge--idle">{{ promptState.source || 'default' }}</span>
        </div>
      </div>

      <div class="prompt-layout">
        <article class="prompt-card">
          <div class="prompt-card__titlebar">
            <div>
              <h4>Base prompt editor</h4>
              <p>Use placeholders to inject labeled Butler context sections.</p>
            </div>
            <label class="prompt-toggle">
              <input v-model="promptEnabled" type="checkbox">
              <span>Use operator prompt</span>
            </label>
          </div>

          <textarea
            v-model="promptDraft"
            class="prompt-textarea"
            rows="12"
            placeholder="Write the operator base prompt here"
          />

          <div class="prompt-toolbar">
            <div class="prompt-stats">
              <span>{{ promptDraft.length }} chars</span>
              <span :class="promptBudgetClass">{{ promptBudgetLabel }}</span>
              <span v-if="promptState.updated_at">Updated {{ formatDate(promptState.updated_at) }}</span>
              <span v-if="promptState.updated_by">by {{ promptState.updated_by }}</span>
            </div>
            <div class="prompt-actions">
              <button class="provider-btn provider-btn--ghost" type="button" :disabled="promptBusy" @click="resetPromptDraft">Reset</button>
              <button class="provider-btn provider-btn--ghost" type="button" :disabled="promptBusy || previewBusy" @click="runPromptPreview">Preview</button>
              <button class="provider-btn" type="button" :disabled="promptBusy" @click="savePrompt">Save prompt</button>
            </div>
          </div>

          <p v-if="promptWarning" class="prompt-warning">{{ promptWarning }}</p>

          <div class="placeholder-grid">
            <button
              v-for="placeholder in promptState.available_placeholders"
              :key="placeholder"
              class="placeholder-chip"
              type="button"
              :disabled="promptBusy"
              @click="insertPlaceholder(placeholder)"
            >
              {{ placeholder }}
            </button>
          </div>
        </article>

        <article class="prompt-card prompt-card--preview">
          <div class="prompt-card__titlebar">
            <div>
              <h4>Effective prompt preview</h4>
              <p>Preview with safe section labels and tool capability summary.</p>
            </div>
          </div>

          <div class="prompt-preview-controls">
            <input v-model="previewSessionKey" class="provider-input" type="text" placeholder="Session key for preview (optional)">
            <textarea v-model="previewUserMessage" class="provider-textarea" rows="3" placeholder="User message to test context retrieval (optional)" />
          </div>

          <p v-if="previewData?.unknown_placeholders?.length" class="prompt-warning">
            Unknown placeholders: {{ previewData.unknown_placeholders.join(', ') }}
          </p>
          <p v-if="previewData?.truncated" class="prompt-warning">Preview was truncated to stay within the current prompt budget.</p>

          <div class="preview-sections">
            <article v-for="section in visiblePreviewSections" :key="section.name" class="preview-section">
              <div class="preview-section__header">
                <strong>{{ section.label }}</strong>
                <span>{{ section.inserted ? 'Inserted' : 'Appended' }}</span>
              </div>
              <pre>{{ section.content }}</pre>
            </article>
          </div>

          <div class="preview-final">
            <div class="preview-section__header">
              <strong>Final prompt</strong>
              <span>{{ previewData?.final_prompt?.length ?? 0 }} chars</span>
            </div>
            <pre>{{ previewData?.final_prompt || promptState.effective_prompt }}</pre>
          </div>
        </article>
      </div>
    </div>

    <p v-if="!pending && allGroups.length && !groups.length" class="placeholder-text">No settings match the current filters.</p>

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
</div></template>

<script setup lang="ts">
const mindPanelProps=defineProps<{ownerToken?:string}>()
import { computed, ref } from 'vue'
import SettingsGroup from '~/features/mind/controls/SettingsGroup.vue'
import { useSettingsData, type PromptConfig, type PromptPreview, type ProviderItem, type SettingItem, type SettingsComponent } from '~/composables/useSettings'

const ownerToken = ref(mindPanelProps.ownerToken || '')
onBeforeUnmount(() => { ownerToken.value = '' })
const { data, pending, error, refresh, updateSetting, deleteSetting, getRestartState, applyRestart, getProviders, startProviderAuth, completeProviderAuth, deleteProviderAuth, getPrompt, updatePrompt, previewPrompt } = useSettingsData(ownerToken)
const busyKey = ref<string | null>(null)
const providerBusy = ref<string | null>(null)
const toast = ref('')
const restartKeys = ref(new Set<string>())
const restartBusy = ref(false)
const restartState = ref({ components: [] as string[], suggested_command: '' })
const providerState = ref({ active_provider: '', providers: [] as ProviderItem[] })
const enterpriseInput = ref('')
const codexInput = ref('')
const promptBusy = ref(false)
const previewBusy = ref(false)
const promptState = ref<PromptConfig>({ configured_prompt: '', effective_prompt: '', enabled: true, source: 'default', available_placeholders: [] })
const promptDraft = ref('')
const promptEnabled = ref(true)
const previewSessionKey = ref('')
const previewUserMessage = ref('')
const previewData = ref<PromptPreview | null>(null)
const settingsQuery = ref('')
const sourceFilter = ref<'all' | 'env' | 'db' | 'default'>('all')
const onlyRestart = ref(false)
const focusSegment = ref<'all' | 'policy' | 'runtime' | 'connectivity'>('all')

const allGroups = computed<SettingsComponent[]>(() => data.value ?? [])
const focusSegments = [
  { value: 'all', label: 'All settings' },
  { value: 'policy', label: 'Policy focus' },
  { value: 'runtime', label: 'Runtime focus' },
  { value: 'connectivity', label: 'Provider/connectivity' },
] as const

const policyKeyPattern = /(policy|approval|autonomy|memory|guard|prompt)/i
const runtimeKeyPattern = /(timeout|retry|queue|worker|concurrency|schedule|doctor|health)/i
const connectivityKeyPattern = /(provider|oauth|token|auth|model|github|openai|copilot)/i

const groups = computed<SettingsComponent[]>(() => {
  const query = settingsQuery.value.trim().toLowerCase()

  const filterBySegment = (setting: SettingItem): boolean => {
    if (focusSegment.value === 'policy') {
      return policyKeyPattern.test(setting.key) || policyKeyPattern.test(setting.group) || policyKeyPattern.test(setting.component)
    }
    if (focusSegment.value === 'runtime') {
      return runtimeKeyPattern.test(setting.key) || runtimeKeyPattern.test(setting.group) || runtimeKeyPattern.test(setting.component)
    }
    if (focusSegment.value === 'connectivity') {
      return connectivityKeyPattern.test(setting.key) || connectivityKeyPattern.test(setting.group) || connectivityKeyPattern.test(setting.component)
    }
    return true
  }

  return allGroups.value
    .map((group) => {
      const settings = group.settings.filter((setting) => {
        const bySource = sourceFilter.value === 'all' || setting.source === sourceFilter.value
        const byRestart = !onlyRestart.value || setting.requires_restart
        const bySegment = filterBySegment(setting)
        const byQuery = !query || [setting.key, setting.group, setting.component].join(' ').toLowerCase().includes(query)
        return bySource && byRestart && bySegment && byQuery
      })
      return { ...group, settings }
    })
    .filter((group) => group.settings.length > 0)
})

const totalSettingsCount = computed(() => allGroups.value.reduce((acc, group) => acc + group.settings.length, 0))
const filteredSettingsCount = computed(() => groups.value.reduce((acc, group) => acc + group.settings.length, 0))

const policyHighlights = computed<SettingItem[]>(() => {
  const ranked = allGroups.value
    .flatMap((group) => group.settings)
    .filter((setting) => policyKeyPattern.test(setting.key) || setting.validation_status !== 'valid' || setting.requires_restart)
    .sort((left, right) => {
      const leftRank = (left.validation_status !== 'valid' ? 2 : 0) + (left.requires_restart ? 1 : 0)
      const rightRank = (right.validation_status !== 'valid' ? 2 : 0) + (right.requires_restart ? 1 : 0)
      return rightRank - leftRank
    })

  return ranked.slice(0, 8)
})

const promptBudgetLabel = computed(() => promptDraft.value.length > 6000 ? 'Over guidance' : promptDraft.value.length > 4000 ? 'Near limit' : 'Within guidance')
const promptBudgetClass = computed(() => promptDraft.value.length > 6000 ? 'prompt-budget prompt-budget--danger' : promptDraft.value.length > 4000 ? 'prompt-budget prompt-budget--warn' : 'prompt-budget')
const promptWarning = computed(() => promptDraft.value.length > 6000 ? 'Prompt is getting large. Consider moving details into placeholders and runtime memory sections.' : '')
const visiblePreviewSections = computed(() => (previewData.value?.sections ?? []).filter((section) => !section.omitted))
const runtimeConfig = useRuntimeConfig()

const extensionRemoteURL = computed(() => {
  const base = String(runtimeConfig.public.apiBase ?? '').trim()
  if (!base) {
    return ''
  }
  return base.replace(/\/+$/g, '')
})

function settingValueByKey(key: string): string {
  for (const group of allGroups.value) {
    const setting = group.settings.find((item) => item.key === key)
    if (setting) {
      return String(setting.value ?? '').trim()
    }
  }
  return ''
}

const browserControl = computed(() => {
  const transportMode = settingValueByKey('BUTLER_SINGLE_TAB_TRANSPORT_MODE') || 'dual'
  const heartbeatTTL = settingValueByKey('BUTLER_SINGLE_TAB_RELAY_HEARTBEAT_TTL_SECONDS') || '90'
  const extensionAuthConfigured = settingValueByKey('BUTLER_EXTENSION_API_TOKENS') !== ''
  return {
    transportMode,
    heartbeatTTL,
    extensionAuthConfigured,
  }
})

function replaceSetting(nextSetting: SettingItem) {
  const nextGroups = (data.value ?? []).map((group) => ({
    ...group,
    settings: group.settings.map((setting) => setting.key === nextSetting.key ? nextSetting : setting),
  }))
  if (!nextGroups.some((group) => group.settings.some((setting) => setting.key === nextSetting.key))) {
    nextGroups.push({ name: nextSetting.component, settings: [nextSetting] })
  }
  data.value = nextGroups.sort((a, b) => a.name.localeCompare(b.name))
}

function extractErrorMessage(err: unknown, fallback: string): string {
  if (typeof err === 'object' && err !== null) {
    const candidate = err as { data?: { error?: string }, message?: string }
    if (candidate.data?.error) {
      return candidate.data.error
    }
    if (candidate.message) {
      return candidate.message
    }
  }
  return fallback
}

async function saveSetting(payload: { key: string, value: string }) {
  busyKey.value = payload.key
  toast.value = ''
  try {
    const setting = await updateSetting(payload.key, payload.value)
    replaceSetting(setting)
    await refreshRestartState()
  } catch (err: unknown) {
    toast.value = extractErrorMessage(err, 'Failed to save setting.')
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
  } catch (err: unknown) {
    toast.value = extractErrorMessage(err, 'Failed to delete setting.')
  } finally {
    busyKey.value = null
  }
}

async function refreshRestartState() {
  const payload = await getRestartState()
  restartState.value = payload
  const activeKeys = new Set<string>()
  const componentSet = new Set(payload.components)
  for (const group of allGroups.value) {
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
  await refreshPrompt()
}

function providerTitle(name: string) {
  switch (name) {
    case 'antigravity':
      return 'Google Antigravity · Google AI Pro'
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
  } catch (err: unknown) {
    toast.value = extractErrorMessage(err, 'Failed to start provider login.')
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
  } catch (err: unknown) {
    toast.value = extractErrorMessage(err, 'Failed to complete provider login.')
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
  } catch (err: unknown) {
    toast.value = extractErrorMessage(err, 'Failed to disconnect provider.')
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
    if (payload.accepted && payload.services?.length) {
      toast.value = `Restart scheduled for ${payload.services.join(', ')}. Butler may be briefly unavailable while services come back.`
    } else if (payload.suggested_command) {
      toast.value = `Run command to restart changed services: ${payload.suggested_command}`
    }
  } catch (err: unknown) {
    toast.value = extractErrorMessage(err, 'Failed to schedule restart.')
  } finally {
    restartBusy.value = false
  }
}

async function refreshPrompt() {
	promptState.value = await getPrompt()
	promptDraft.value = promptState.value.configured_prompt || promptState.value.effective_prompt
	promptEnabled.value = promptState.value.enabled
	if (!previewData.value) {
	  previewData.value = {
		prompt: promptState.value,
		configured_prompt: promptState.value.configured_prompt,
		effective_base_prompt: promptState.value.effective_prompt,
		final_prompt: promptState.value.effective_prompt,
		truncated: false,
		sections: [],
	  }
	}
}

async function savePrompt() {
	promptBusy.value = true
	toast.value = ''
	try {
	  promptState.value = await updatePrompt(promptDraft.value, promptEnabled.value)
	  promptDraft.value = promptState.value.configured_prompt || promptState.value.effective_prompt
	  previewData.value = null
	  await runPromptPreview()
	  toast.value = 'Prompt settings saved.'
	} catch (err: unknown) {
	  toast.value = extractErrorMessage(err, 'Failed to save prompt.')
	} finally {
	  promptBusy.value = false
	}
}

async function runPromptPreview() {
	previewBusy.value = true
	toast.value = ''
	try {
	  previewData.value = await previewPrompt(previewSessionKey.value, previewUserMessage.value)
	} catch (err: unknown) {
	  toast.value = extractErrorMessage(err, 'Failed to preview prompt.')
	} finally {
	  previewBusy.value = false
	}
}

function resetPromptDraft() {
	promptDraft.value = promptState.value.configured_prompt || promptState.value.effective_prompt
	promptEnabled.value = promptState.value.enabled
}

function insertPlaceholder(placeholder: string) {
	if (!promptDraft.value.includes(placeholder)) {
	  promptDraft.value = [promptDraft.value.trim(), placeholder].filter(Boolean).join('\n\n')
	}
}

await refreshProviders()
await refreshRestartState()
await refreshPrompt()
await runPromptPreview()
</script>
