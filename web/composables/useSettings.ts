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

export interface ToolsRegistryState {
  path: string
  content: string
}

export interface RestartState {
  components: string[]
  suggested_command: string
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

  async function getToolsRegistry(): Promise<ToolsRegistryState> {
    return await $fetch<ToolsRegistryState>(`${baseURL}/api/v1/settings/tools-registry`)
  }

  async function updateToolsRegistry(content: string): Promise<{ updated: boolean, path: string }> {
    return await $fetch<{ updated: boolean, path: string }>(`${baseURL}/api/v1/settings/tools-registry`, {
      method: 'PUT',
      body: { content },
    })
  }

  async function getRestartState(): Promise<RestartState> {
    return await $fetch<RestartState>(`${baseURL}/api/v1/settings/restart`)
  }

  async function applyRestart(): Promise<RestartState> {
    return await $fetch<RestartState>(`${baseURL}/api/v1/settings/restart`, {
      method: 'POST',
    })
  }

  return {
    ...data,
    updateSetting,
    deleteSetting,
    getToolsRegistry,
    updateToolsRegistry,
    getRestartState,
    applyRestart,
  }
}
