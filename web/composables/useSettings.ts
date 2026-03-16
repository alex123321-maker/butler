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
  }
}
