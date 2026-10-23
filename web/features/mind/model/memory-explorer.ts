import type { Source } from './types'

export interface MemoryEdge {
  from: string
  to: string
  label: string
}
export function readableSource(source: Source): boolean {
  return (
    source.status === 'active' &&
    source.origin?.sensitivity !== 'secret' &&
    !source.redacted
  )
}
export function sourceParents(source: Source): string[] {
  if (!readableSource(source)) return []
  return [...new Set(source.origin?.parent_refs || [])].filter(
    (id) => id !== source.source_id,
  )
}
// Provenance is historical DATA. Neither a shared topic nor a common scope is a link.
export function memoryEdges(sources: Source[]): MemoryEdge[] {
  const ids = new Set(sources.filter(readableSource).map((s) => s.source_id))
  return sources.flatMap((s) =>
    sourceParents(s)
      .filter((id) => ids.has(id))
      .map((id) => ({
        from: s.source_id,
        to: id,
        label: 'Происхождение',
      })),
  )
}
export function sourceStatus(source: Source): string {
  if (source.origin?.sensitivity === 'secret' || source.redacted)
    return 'Закрытое содержимое'
  return (
    (
      {
        active: 'Источник сохранён',
        revoked: 'Доступ отозван',
        deleted: 'Источник удалён',
        quarantined: 'Нужна проверка источника',
      } as Record<string, string>
    )[source.status] || 'Состояние неизвестно'
  )
}
