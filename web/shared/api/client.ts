export type QueryValue = string | number | boolean | undefined | null

export interface ApiRequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  query?: Record<string, QueryValue>
  body?: unknown
}

const buildQuery = (query?: Record<string, QueryValue>): string => {
  if (!query) {
    return ''
  }

  const params = new URLSearchParams()

  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null || value === '') {
      continue
    }
    params.set(key, String(value))
  }

  const encoded = params.toString()
  return encoded ? `?${encoded}` : ''
}

export const apiRequest = async <T>(path: string, options: ApiRequestOptions = {}): Promise<T> => {
  const config = useRuntimeConfig()
  const baseUrl = String(config.public.apiBase || '').replace(/\/$/, '')
  const query = buildQuery(options.query)

  return await $fetch<T>(`${baseUrl}${path}${query}`, {
    method: options.method ?? 'GET',
    body: options.body,
    headers: {
      'Content-Type': 'application/json',
    },
  })
}
