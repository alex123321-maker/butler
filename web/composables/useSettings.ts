import { useApiClient } from './useApi'

export interface SettingItem {
  key: string
  component: string
  value: string
  source: 'env' | 'db' | 'default' | string
  is_secret: boolean
  requires_restart: boolean
  validation_status: string
  validation_error?: string
}

export interface SettingsComponent {
  name: string
  settings: SettingItem[]
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

  return {
    ...data,
    updateSetting,
    deleteSetting,
  }
}
