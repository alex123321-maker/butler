interface HealthResponse {
  status: string
  healthy: boolean
}

export function useApiClient() {
  const config = useRuntimeConfig()
  const baseURL = config.public.apiBase as string

  async function get<T>(path: string): Promise<T> {
    return await $fetch<T>(`${baseURL}${path}`)
  }

  async function post<T>(path: string, body?: unknown): Promise<T> {
    return await $fetch<T>(`${baseURL}${path}`, { method: 'POST', body })
  }

  return { get, post, baseURL }
}

export function useHealthCheck() {
  const { baseURL } = useApiClient()

  return useAsyncData<HealthResponse>('health', async () => {
    try {
      const resp = await $fetch<HealthResponse>(`${baseURL}/health`)
      return { status: resp.status || 'ok', healthy: true }
    } catch {
      return { status: 'unreachable', healthy: false }
    }
  }, {
    server: false,
    lazy: true,
  })
}
