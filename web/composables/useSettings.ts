import { useApiClient } from './useApi'

export interface SettingItem {
  key: string
  component: string
  group: string
  value: string
  source: 'env' | 'db' | 'default' | string
  is_secret: boolean
  requires_restart: boolean
  allowed_values?: string[]
  validation_status: string
  validation_error?: string
}

export interface SettingsComponent {
  name: string
  settings: SettingItem[]
}

export interface RestartState {
  components: string[]
  suggested_command: string
}

export interface ProviderFlowState {
  id: string
  status: string
  verification_uri?: string
  user_code?: string
  auth_url?: string
  instructions?: string
  expires_at?: string
  error?: string
}

export interface ProviderItem {
  name: string
  model: string
  active: boolean
  connected: boolean
  auth_kind: string
  account_hint?: string
  enterprise_domain?: string
  expires_at?: string
  pending?: ProviderFlowState | null
}

export interface ProvidersState {
  active_provider: string
  providers: ProviderItem[]
}

export interface PromptConfig {
  configured_prompt: string
  effective_prompt: string
  enabled: boolean
  source: 'env' | 'db' | 'default' | string
  updated_at?: string
  updated_by?: string
  available_placeholders: string[]
}

export interface PromptSection {
  name: string
  label: string
  content: string
  inserted: boolean
  truncated: boolean
  omitted: boolean
  omitted_reason?: string
}

export interface PromptPreview {
  prompt: PromptConfig
  configured_prompt: string
  effective_base_prompt: string
  final_prompt: string
  unknown_placeholders?: string[]
  truncated: boolean
  sections: PromptSection[]
}

export function useSettingsData() {
  const { get, baseURL } = useApiClient()

  const data = useAsyncData<SettingsComponent[]>('settings', async () => {
    const response = await get<{ components: SettingsComponent[] }>('/api/v1/settings')
    return response.components
  }, {
    server: false,
    default: () => [],
  })

  async function updateSetting(key: string, value: string): Promise<SettingItem> {
    const response = await $fetch<{ setting: SettingItem }>(`${baseURL}/api/v1/settings/${key}`, {
      method: 'PUT',
      body: { value },
    })
    return response.setting
  }

  async function deleteSetting(key: string): Promise<SettingItem> {
    const response = await $fetch<{ setting: SettingItem }>(`${baseURL}/api/v1/settings/${key}`, {
      method: 'DELETE',
    })
    return response.setting
  }

  async function getRestartState(): Promise<RestartState> {
    return await $fetch<RestartState>(`${baseURL}/api/v1/settings/restart`)
  }

  async function applyRestart(): Promise<RestartState> {
    return await $fetch<RestartState>(`${baseURL}/api/v1/settings/restart`, {
      method: 'POST',
    })
  }

  async function getProviders(): Promise<ProvidersState> {
    return await get<ProvidersState>('/api/v1/providers')
  }

  async function getProvider(name: string): Promise<ProviderItem> {
    const response = await get<{ provider: ProviderItem }>(`/api/v1/providers/${name}/auth`)
    return response.provider
  }

  async function startProviderAuth(name: string, enterpriseURL = ''): Promise<{ provider: ProviderItem, flow?: ProviderFlowState }> {
    return await $fetch<{ provider: ProviderItem, flow?: ProviderFlowState }>(`${baseURL}/api/v1/providers/${name}/auth/start`, {
      method: 'POST',
      body: { enterprise_url: enterpriseURL },
    })
  }

  async function completeProviderAuth(name: string, flowId: string, input: string): Promise<ProviderItem> {
    const response = await $fetch<{ provider: ProviderItem }>(`${baseURL}/api/v1/providers/${name}/auth/complete`, {
      method: 'POST',
      body: { flow_id: flowId, input },
    })
    return response.provider
  }

  async function deleteProviderAuth(name: string): Promise<ProviderItem> {
    const response = await $fetch<{ provider: ProviderItem }>(`${baseURL}/api/v1/providers/${name}/auth`, {
      method: 'DELETE',
    })
    return response.provider
  }

  async function getPrompt(): Promise<PromptConfig> {
	  const response = await get<{ prompt: PromptConfig }>('/api/v1/prompts/system')
	  return response.prompt
	}

	async function updatePrompt(basePrompt: string, enabled: boolean): Promise<PromptConfig> {
	  const response = await $fetch<{ prompt: PromptConfig }>(`${baseURL}/api/v1/prompts/system`, {
		method: 'PUT',
		body: { base_prompt: basePrompt, enabled },
	  })
	  return response.prompt
	}

	async function previewPrompt(sessionKey: string, userMessage: string): Promise<PromptPreview> {
	  const response = await $fetch<{ preview: PromptPreview }>(`${baseURL}/api/v1/prompts/system/preview`, {
		method: 'POST',
		body: { session_key: sessionKey, user_message: userMessage },
	  })
	  return response.preview
	}

  return {
    ...data,
    updateSetting,
    deleteSetting,
    getRestartState,
    applyRestart,
    getProviders,
    getProvider,
    startProviderAuth,
    completeProviderAuth,
    deleteProviderAuth,
	getPrompt,
	updatePrompt,
	previewPrompt,
  }
}
