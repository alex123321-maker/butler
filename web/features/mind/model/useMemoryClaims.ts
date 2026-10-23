import { useMind } from './store'
import type { MemoryClaim } from './types'
import { readableSource } from './memory-explorer'

// Never cache an owner decision across a source selection or authority change.
export function useMemoryClaims() {
  const store = useMind()
  const catalog = ref<MemoryClaim[]>([]),
    busy = ref(false),
    error = ref(''),
    deciding = ref(false)
  let version = 0,
    alive = true
  const claims = computed(() =>
    catalog.value.filter(
      (c) =>
        c.source_id === store.memorySourceId &&
        c.source_revision === store.inspection?.source.revision,
    ),
  )
  async function load() {
    const requestVersion = ++version,
      epoch = store.epoch
    const current = () =>
      alive && epoch === store.epoch && requestVersion === version
    catalog.value = []
    error.value = ''
    busy.value = true
    try {
      const result = await store.request<{
        claims: MemoryClaim[]
        claim_list_limit: number
      }>('/api/v4/memory')
      if (!current()) return
      catalog.value = result.claims || []
      if (catalog.value.length >= result.claim_list_limit)
        error.value =
          'Показана часть фактов. Полный список доступен в управлении памятью.'
    } catch {
      if (current()) error.value = 'Не удалось проверить факты.'
    } finally {
      if (current()) busy.value = false
    }
  }
  async function decide(claim: MemoryClaim, reject: boolean) {
    if (
      deciding.value ||
      busy.value ||
      error.value ||
      claim.status !== 'candidate' ||
      !claims.value.some((c) => c.claim_id === claim.claim_id)
    )
      return
    const epoch = store.epoch,
      selection = version
    const current = () =>
      alive && epoch === store.epoch && selection === version
    const active = catalog.value.find(
      (c) =>
        c.status === 'confirmed' &&
        c.key === claim.key &&
        c.scope_id === claim.scope_id &&
        c.scope_type === claim.scope_type,
    )
    deciding.value = true
    try {
      await store.request('/api/v4/memory/decisions', {
        claim_id: claim.claim_id,
        expected_active_id: active?.claim_id || '',
        reject,
      })
      if (current()) await load()
    } catch {
      if (current())
        error.value =
          'Решение не подтверждено. Проверьте состояние перед повтором.'
    } finally {
      if (alive && epoch === store.epoch) deciding.value = false
    }
  }
  watch(
    () => store.memorySourceId,
    () => {
      version++
      catalog.value = []
      error.value = ''
      busy.value = false
    },
  )
  watch(
    () => store.inspection,
    (value) => {
      if (value && readableSource(value.source)) void load()
    },
    { immediate: true },
  )
  onBeforeUnmount(() => {
    alive = false
    version++
  })
  return { claims, busy, error, deciding, decide }
}
