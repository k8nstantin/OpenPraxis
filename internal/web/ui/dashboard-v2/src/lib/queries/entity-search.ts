import type { EntityKind } from './entity'

export type EntityHit = {
  kind: EntityKind
  id: string
  title: string
}

type Row = {
  entity_uid?: string
  id?: string
  title?: string
  description?: string
  status?: string
  type?: string
}

// searchOne — search a single entity kind via the unified
// /api/entities/search endpoint.
async function searchOne(kind: EntityKind, query: string): Promise<EntityHit[]> {
  const url = `/api/entities/search?q=${encodeURIComponent(query)}&type=${kind}&limit=8`
  try {
    const res = await fetch(url)
    if (!res.ok) return []
    const data = (await res.json()) as Row[] | { results?: Row[] }
    const rows: Row[] = Array.isArray(data) ? data : (data.results ?? [])
    return rows
      .filter((r) => r && (r.entity_uid ?? r.id))
      .map((r) => ({
        kind,
        id: r.entity_uid ?? r.id ?? '',
        title: r.title ?? (r.entity_uid ?? r.id ?? '').slice(0, 8),
      }))
  } catch {
    return []
  }
}

// searchEntities — fan-out to all three entity-search endpoints via the
// unified /api/entities/search. Used by the BlockNote `@`-mention picker
// + the slash-menu "Link to …" items. Empty query returns empty.
export async function searchEntities(query: string): Promise<EntityHit[]> {
  const q = query.trim()
  if (!q) return []
  const [p, m, t] = await Promise.all([
    searchOne('product', q),
    searchOne('manifest', q),
    searchOne('task', q),
  ])
  return [...p, ...m, ...t]
}

export function entityHref(hit: EntityHit): string {
  const slug =
    hit.kind === 'product'
      ? 'products'
      : hit.kind === 'manifest'
        ? 'manifests'
        : 'tasks'
  return `/${slug}/${hit.id}`
}

export async function getEntityTitle(
  kind: EntityKind,
  id: string
): Promise<string | null> {
  try {
    const res = await fetch(`/api/entities/${id}`)
    if (!res.ok) return null
    const data = (await res.json()) as Row
    return data.title ?? null
  } catch {
    return null
  }
}
